package frame

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/pitabwire/frame/framefakes"
	"gocloud.dev/pubsub"
	"net/http"

	//_ "gocloud.dev/pubSub/awssnssqs"
	//_ "gocloud.dev/pubSub/azuresb"
	_ "gocloud.dev/pubsub/gcppubsub"
	//_ "gocloud.dev/pubSub/kafkapubsub"
	//_ "gocloud.dev/pubSub/natspubsub"
	//_ "gocloud.dev/pubSub/rabbitpubsub"
	"google.golang.org/grpc/test/bufconn"
	"log"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestService_RegisterPublisherNotSet(t *testing.T) {
	ctx := context.Background()

	srv := NewService("Test Srv")

	err := srv.Publish(ctx, "random", []byte(""))

	if err == nil {
		t.Errorf("We shouldn't be able to publish when no topic was registered")
	}

}

func TestService_RegisterPublisherNotInitialized(t *testing.T) {
	ctx := context.Background()
	opt := RegisterPublisher("test", "mem://topicA")
	srv := NewService("Test Srv", opt)

	err := srv.Publish(ctx, "random", []byte(""))

	if err == nil {
		t.Errorf("We shouldn't be able to publish when no topic was registered")
	}

}

func TestService_RegisterPublisher(t *testing.T) {
	ctx := context.Background()

	opt := RegisterPublisher("test", "mem://topicA")
	srv := NewService("Test Srv", opt)

	err := srv.initPubsub(ctx)
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %+v", err)
		return
	}

	err = srv.Publish(ctx, "test", []byte(""))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %+v", err)
	}

	srv.Stop()

}

func TestService_RegisterPublisherMultiple(t *testing.T) {
	ctx := context.Background()

	opt := RegisterPublisher("test", "mem://topicA")
	opt1 := RegisterPublisher("test-2", "mem://topicB")
	srv := NewService("Test Srv", opt, opt1)

	err := srv.initPubsub(ctx)
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %+v", err)
		return
	}

	err = srv.Publish(ctx, "test", []byte("Testament"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %+v", err)
	}

	err = srv.Publish(ctx, "test-2", []byte("Testament"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %+v", err)
	}

	err = srv.Publish(ctx, "test-3", []byte("Testament"))
	if err == nil {
		t.Errorf("We should not be able to publish to topic that was not registered")
	}

}

type messageHandler struct {
}

func (m *messageHandler) Handle(ctx context.Context, message []byte) error {

	log.Printf(" A nice message to handle: %v", string(message))
	return nil
}

type handlerWithError struct {
}

func (m *handlerWithError) Handle(ctx context.Context, message []byte) error {

	log.Printf(" A dreadful message to handle: %v", string(message))
	return errors.New("throwing an error for tests")

}

func TestService_RegisterSubscriber(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	optTopic := RegisterPublisher("test", "mem://topicA")
	opt := RegisterSubscriber("test", "mem://topicA",
		5, &messageHandler{})

	srv := NewService("Test Srv", optTopic, opt)

	err := srv.initPubsub(ctx)
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %+v", err)
		return
	}

	for i := range make([]int, 30) {
		err = srv.Publish(ctx, "test", []byte(fmt.Sprintf(" testing message %d", i)))
		if err != nil {
			t.Errorf("We could not publish to topic that was registered %+v", err)
		}

	}

	err = srv.Publish(ctx, "test", []byte("throw error"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %+v", err)
	}
	srv.Stop()
}

func TestService_RegisterSubscriberWithError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	optTopic := RegisterPublisher("test", "mem://topicA")
	opt := RegisterSubscriber("test", "mem://topicA", 1, &handlerWithError{})

	srv := NewService("Test Srv", optTopic, opt)

	err := srv.initPubsub(ctx)
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %+v", err)
		return
	}

	err = srv.Publish(ctx, "test", []byte(fmt.Sprintf(" testing message with error")))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %+v", err)
	}
	srv.Stop()
}

func TestService_RegisterSubscriberInvalid(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opt := RegisterSubscriber("test", "memt+://topicA",
		5, &messageHandler{})

	srv := NewService("Test Srv", opt)

	err := srv.initPubsub(ctx)
	if err == nil {
		t.Errorf("We somehow instantiated an invalid subscription ")
	}
}

func TestService_RegisterSubscriberContextCancelWorks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	optTopic := RegisterPublisher("test", "mem://topicA")
	opt := RegisterSubscriber("test", "mem://topicA",
		5, &messageHandler{})

	srv := NewService("Test Srv", opt, optTopic)

	err := srv.initPubsub(ctx)
	if err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	if !srv.queue.subscriptionQueueMap["test"].isInit {
		t.Errorf("Subscription is invalid yet it should be ok")
	}

	cancel()
	time.Sleep(3 * time.Second)

	if srv.queue.subscriptionQueueMap["test"].isInit {
		t.Errorf("Subscription is valid yet it should not be ok")
	}

}

func TestPublishCloudEvent(t *testing.T) {
	/*
		TODO:
		- multiple publishers with same queueUrl?
		- multiple publishers with different topic?

		- multiple subscribers with same queueName;
	*/
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bufferSize := 1024 * 1024
	listener := bufconn.Listen(bufferSize)

	optSubscriber := RegisterSubscriber("test", "gcppubsub://projects/myproject/topics/mytopic",
		5, &messageHandler{})
	optPublisher := RegisterPublisher("test", "gcppubsub://projects/myproject/topics/mytopic")

	q, err := newQueue()
	if err != nil {
		t.Errorf("We somehow fail to instantiate queue: %v", err)
	}

	//1-1 pub-sub
	srv := NewService("Test Srv", RegisterQueue(q), optSubscriber, optPublisher, ServerListener(listener))
	err = srv.initPubsub(ctx)
	if err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	// it is here to properly stop the server
	defer func() { time.Sleep(100 * time.Millisecond) }()
	defer srv.Stop()

	var wg sync.WaitGroup
	go func() {
		wg.Add(1)
		srv.AddPreStartMethod(func(s *Service) {
			go func() {
				time.Sleep(1 * time.Second)
				defer wg.Done()
			}()
		})
		_ = srv.Run(ctx, "")
	}()

	wg.Wait()

	err = srv.Publish(ctx, "test", []byte("Testament"))
	if err != nil {
		t.Fatalf("failed to dial: %+v", err)
	}


}

func TestReceiveCloudEvents(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bufferSize := 1024 * 1024
	listener := bufconn.Listen(bufferSize)

	opt := RegisterSubscriber("test", "http://0.0.0.0/queue/topicA",
		5, &messageHandler{})

	srv := NewService("Test Srv", opt, ServerListener(listener))

	err := srv.initPubsub(ctx)
	if err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	// it is here to properly stop the server
	defer func() { time.Sleep(100 * time.Millisecond) }()
	defer srv.Stop()

	go func() {
		_ = srv.Run(ctx, "")
	}()

	err = clientInvokeWithCloudEventPayload(listener)
	if err != nil {
		t.Fatalf("failed to dial: %+v", err)
	}
}

func clientInvokeWithCloudEventPayload(listener *bufconn.Listener) error {
	ev := newCloudEvent("com.cloudevents.test.sent", "https://frame/sender/queue",
		map[string]interface{}{
			"id":      "testing test",
			"message": "Hello, World!",
		})
	_, err := json.Marshal(ev)
	if err != nil {
		return err
	}

	//TODO test that the queue will pickup cloudevents

	return nil

}

func newCloudEvent(eventType, eventSource string, eventData interface{}) cloudevents.Event {
	ret := cloudevents.NewEvent()
	ret.SetType(eventType)
	ret.SetSource(eventSource)
	_ = ret.SetData(cloudevents.ApplicationJSON, eventData)
	return ret
}

func TestService_Publish(t *testing.T) {
	type fields struct {
		name    string
		queue   *queue
		options []Option
	}
	type args struct {
		reference string
		payload   interface{}
	}
	type exps struct {
		err error
	}

	bufferSize := 1024 * 1024
	listener := bufconn.Listen(bufferSize)

	_ = RegisterSubscriber("test", "gcppubsub://projects/myproject/topics/mytopicA",
		5, &messageHandler{})
	optPublisherA := RegisterPublisher("test", "gcppubsub://projects/myproject/topics/mytopicA")
	optPublisherB := RegisterPublisher("testB", "gcppubsub://projects/myproject/topics/mytopicB")

	q, err := newQueue()
	if err != nil {
		t.Errorf("We somehow fail to instantiate queue: %v", err)
	}

	tests := []struct {
		name    string
		fields  fields
		args    args
		exps    exps
		wantErr bool
	}{
		{
			"should publish event with 1 publisher, no subscriber",
			fields{
				name:    "Test Srv(1_publisher)",
				queue:   q,
				options: []Option{RegisterQueue(q), optPublisherA, ServerListener(listener)},
			},
			args{
				reference: "test",
				payload:   []byte("Testament"),
			},
			exps{
				nil,
			},
			false,
		}, {
			"shouldn't publish event without publishers",
			fields{
				name:    "Test Srv",
				queue:   q,
				options: []Option{RegisterQueue(q), ServerListener(listener)},
			},
			args{
				reference: "unknown",
				payload:   []byte("Testament"),
			},
			exps{
				err: fmt.Errorf(
					"getPublisherByReference -- you need to register a queue : [%v] first before publishing ",
					"unknown"),
			},
			true,
		}, {
			"should publish event with 2 publisher, 1 subscriber",
			fields{
				name:    "Test Srv(2_publishers,1_subscriber)",
				queue:   q,
				options: []Option{RegisterQueue(q), optPublisherA, optPublisherB /*optSubscriber, */, ServerListener(listener)},
			},
			args{
				reference: "test",
				payload:   []byte("Testament"),
			},
			exps{
				nil,
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			mockCEClient := &framefakes.FakeICEClient{}
			mockPSClient := &framefakes.FakeIPSClient{}
			mockCEClient.SendReturns(nil)
			mockPSClient.OpenTopicReturns(&pubsub.Topic{}, nil)
			mockPSClient.OpenSubscriptionReturns(&pubsub.Subscription{}, nil)

			{
				q := tt.fields.queue
				q.WithClient(mockCEClient)
				q.WithPubSub(mockPSClient)
			}

			srv := NewService(tt.fields.name, tt.fields.options...)

			// it is here to properly stop the server
			defer func() { time.Sleep(100 * time.Millisecond) }()
			defer srv.Stop()

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				var err error
				srv.AddPreStartMethod(func(s *Service) {
					go func() {
						time.Sleep(1 * time.Second)
						if err == nil {
							wg.Done()
						}
					}()
				})

				if err = srv.Run(ctx, ":8080"); err != nil && err != http.ErrServerClosed {
					t.Fatalf("srv.Run fail: %v", err)
				}
			}()

			wg.Wait()
			srv.AddCleanupMethod(func() {
				if err := srv.server.Shutdown(context.TODO()); err != nil {
					log.Fatalf("srv.server.Shutdown error: %v", err)
				}
			})

			if err := srv.Publish(ctx, tt.args.reference, tt.args.payload); (err != nil) != tt.wantErr ||
				(tt.wantErr && err.Error() != tt.exps.err.Error()) {
				t.Errorf("Publish() error = %v, expected error %v. wantErr=%v", err, tt.exps.err, tt.wantErr)
				return
			}

			if callCount, expCallCount := mockPSClient.OpenTopicCallCount(), len(srv.queue.publishQueueMap); callCount != expCallCount {
				t.Errorf("expected OpenTopicCallCount to eq %v, but got %v", expCallCount, callCount)
			}

			if tt.wantErr {
				return
			}

			if val := mockCEClient.SendCallCount(); val != 1 {
				t.Errorf("expected SendCallCount to eq 1, but got %v", val)
			}

			expEvent := newCloudEvent(fmt.Sprintf("%s.%T", tt.fields.name, tt.args.payload),
				fmt.Sprintf("%s/%s", tt.fields.name, tt.args.reference), tt.args.payload)
			if arg0, arg1 := mockCEClient.SendArgsForCall(0); arg0 == nil || !reflect.DeepEqual(arg1, expEvent) {
				t.Errorf("expected SendArgsForCall to eq: (!nil, %v), but got(%v, %v)", expEvent, arg0, arg1)
			}
		})
	}
}
