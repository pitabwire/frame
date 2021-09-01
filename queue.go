package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"gocloud.dev/pubsub"
	_ "gocloud.dev/pubsub/mempubsub"
	"log"
	"strings"
)

type queue struct {
	ceClient cloudevents.Client

	publishQueueMap      map[string]*publisher
	subscriptionQueueMap map[string]*subscriber
}

func (q queue) getPublisherByReference(reference string) (*publisher, error) {
	p := q.publishQueueMap[reference]
	if p == nil {
		return nil, errors.New(fmt.Sprintf("getPublisherByReference -- you need to register a queue : [%v] first before publishing ", reference))
	}

	if !p.isInit {
		return nil, errors.New(fmt.Sprintf("getPublisherByReference -- can't publish on uninitialized queue %v ", reference))
	}

	return p, nil
}

func (q queue) getSubscriberByReference(reference string) (*subscriber, error) {
	s := q.subscriptionQueueMap[reference]
	if s == nil {
		return nil, errors.New(fmt.Sprintf("getSubscriberByReference -- you need to register a queue : [%v] first before publishing ", reference))
	}
	return s, nil
}

func newQueue() (*queue, error) {

	cl, err := cloudevents.NewClientHTTP()
	if err != nil {
		return nil, err
	}
	q := &queue{
		ceClient:             cl,
		publishQueueMap:      make(map[string]*publisher),
		subscriptionQueueMap: make(map[string]*subscriber),
	}

	return q, nil

}

type publisher struct {
	reference string
	url       string
	pubTopic  *pubsub.Topic
	isInit    bool
}

func (p publisher) isCloudEvent() bool {
	return strings.HasPrefix(strings.ToLower(p.url), "http")

}

type SubscribeWorker interface {
	Handle(ctx context.Context, message []byte) error
}

type subscriber struct {
	url          string
	concurrency  int
	handler      SubscribeWorker
	subscription *pubsub.Subscription
	isInit       bool
}

// RegisterPublisher Option to register publishing path referenced within the system
func RegisterPublisher(reference string, queueUrl string) Option {
	return func(s *Service) {

		s.queue.publishQueueMap[reference] = &publisher{
			reference: reference,
			url:       queueUrl,
			isInit:    false,
		}
	}
}

// RegisterSubscriber Option to register a new subscription handler
func RegisterSubscriber(reference string, queueUrl string, concurrency int,
	handler SubscribeWorker) Option {
	return func(s *Service) {

		s.queue.subscriptionQueueMap[reference] = &subscriber{
			url:         queueUrl,
			concurrency: concurrency,
			handler:     handler,
		}
	}
}

// QueuePath Option that helps to specify or override the default /queue path to handle cloud events
func QueuePath(path string) Option {
	return func(s *Service) {
		s.queuePath = path
	}
}

// Publish Queue method to write a new message into the queue pre initialized with the supplied reference
func (s Service) Publish(ctx context.Context, reference string, payload interface{}) error {

	var metadata map[string]string

	authClaim := ClaimsFromContext(ctx)
	if authClaim != nil {
		metadata = authClaim.AsMetadata()
	} else {
		metadata = make(map[string]string)
	}

	publisher, err := s.queue.getPublisherByReference(reference)
	if err != nil {
		return err
	}

	if publisher.isCloudEvent() {

		event := cloudevents.NewEvent()
		event.SetSource(fmt.Sprintf("%s/%s", s.Name(), publisher.reference))
		event.SetType(fmt.Sprintf("%s.%T", s.Name(), payload))
		err := event.SetData(cloudevents.ApplicationJSON, payload)
		if err != nil {
			return err
		}
		ctx := cloudevents.ContextWithTarget(ctx, publisher.url)

		result := s.queue.ceClient.Send(ctx, event)
		if cloudevents.IsUndelivered(result) {
			return result
		}

		return nil
	}

	var message []byte
	msg, ok := payload.([]byte)
	if !ok {
		msg, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		message = msg
	} else {
		message = msg
	}

	return publisher.pubTopic.Send(ctx, &pubsub.Message{
		Body:     message,
		Metadata: metadata,
	})

}

func (s Service) initPubsub(ctx context.Context) error {

	if s.queue == nil {
		return nil
	}

	for ref, publisher := range s.queue.publishQueueMap {

		topic, err := pubsub.OpenTopic(ctx, publisher.url)
		if err != nil {
			return err
		}

		s.AddCleanupMethod(func() {
			err := topic.Shutdown(ctx)
			if err != nil {
				log.Printf("initPubsub -- publish topic %s could not be closed : %v", ref, err)
			}
		})

		publisher.pubTopic = topic
		publisher.isInit = true
	}

	for ref, subscriber := range s.queue.subscriptionQueueMap {

		if !strings.HasPrefix(subscriber.url, "http") {

			subs, err := pubsub.OpenSubscription(ctx, subscriber.url)
			if err != nil {
				return fmt.Errorf("could not open topic subscription: %+v", err)
			}

			s.AddCleanupMethod(func() {
				err := subs.Shutdown(ctx)
				if err != nil {
					log.Printf("Subscribe -- subscription %s could not be stopped well : %v", ref, err)
				}
			})

			subscriber.subscription = subs
		}
		subscriber.isInit = true

	}

	if len(s.queue.subscriptionQueueMap) > 0 {
		s.subscribe(ctx)
	}

	return nil

}

func (s Service) subscribe(ctx context.Context) {

	for _, subsc := range s.queue.subscriptionQueueMap {

		// cloud event subscriptions are not held as long running processes
		if strings.HasPrefix(subsc.url, "http") {
			continue
		}

		go func(localSub *subscriber) {

			sem := make(chan struct{}, localSub.concurrency)
		recvLoop:
			for {
				msg, err := localSub.subscription.Receive(ctx)
				if err != nil {
					// Errors from Receive indicate that Receive will no longer succeed.
					log.Printf(" subscribe -- Could not pull message because : %+v", err)
					localSub.isInit = false
					return
				}

				// Wait if there are too many active handle goroutines and acquire the
				// semaphore. If the context is canceled, stop waiting and start shutting
				// down.
				select {
				case sem <- struct{}{}:
				case <-ctx.Done():
					break recvLoop
				}

				go func() {
					defer func() { <-sem }() // Release the semaphore.

					authClaim := ClaimsFromMap(msg.Metadata)

					if nil != authClaim {
						ctx = authClaim.ClaimsToContext(ctx)
					}

					ctx = ToContext(ctx, &s)

					err := localSub.handler.Handle(ctx, msg.Body)
					if err != nil {
						log.Printf(" subscribe -- Unable to process message %v : %v",
							localSub.url, err)
						msg.Nack()
						return
					}

					msg.Ack()

				}()
			}

			// We're no longer receiving messages. Wait to finish handling any
			// unacknowledged messages by totally acquiring the semaphore.
			for n := 0; n < localSub.concurrency; n++ {
				sem <- struct{}{}
			}

			localSub.isInit = false

		}(subsc)

	}

}

func receiveCloudEvents(ctx context.Context, event cloudevents.Event) cloudevents.Result {

	sourceUrl := cloudevents.TargetFromContext(ctx)
	sourcePathList := strings.Split(sourceUrl.Path, "/")

	subscriptionReference := sourcePathList[len(sourcePathList)-1]

	service := FromContext(ctx)
	sub, err := service.queue.getSubscriberByReference(subscriptionReference)
	if err != nil {
		return cloudevents.NewHTTPResult(404, "failed to match subscription due to : %s", err)
	}

	err = sub.handler.Handle(ctx, event.Data())
	if err != nil {
		log.Printf(" receiveCloudEvents -- Unable to process message to %v because : %v", sourceUrl, err)
		return cloudevents.NewHTTPResult(400, "failed to handle inbound request due to : %s", err)
	}

	return nil

}
