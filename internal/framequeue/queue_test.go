package framequeue_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pitabwire/util"

	"github.com/pitabwire/frame"
)

func TestService_RegisterPublisherNotSet(t *testing.T) {
	ctx, srv := frame.NewService("Test Srv")

	err := srv.Publish(ctx, "random", []byte(""))

	if err == nil {
		t.Errorf("We shouldn't be able to publish when no topic was registered")
	}
}

func TestService_RegisterPublisherNotInitialized(t *testing.T) {
	opt := frame.WithRegisterPublisher("test", "mem://topicA")
	ctx, srv := frame.NewService("Test Srv", opt)

	err := srv.Publish(ctx, "random", []byte(""))

	if err == nil {
		t.Errorf("We shouldn't be able to publish when no topic was registered")
	}
}

func TestService_RegisterPublisher(t *testing.T) {
	opt := frame.WithRegisterPublisher("test", "mem://topicA")

	ctx, srv := frame.NewService("Test Srv", opt, frame.WithNoopDriver())

	err := srv.Run(ctx, "")
	if err != nil {
		t.Errorf("we couldn't instantiate queue  %s", err)
		return
	}

	err = srv.Publish(ctx, "test", []byte(""))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %s", err)
	}

	srv.Stop(ctx)
}

func TestService_RegisterPublisherMultiple(t *testing.T) {
	topicRef := "test-multiple-publisher"
	topicRef2 := "test-multiple-publisher-2"

	opt := frame.WithRegisterPublisher(topicRef, "mem://topicA")
	opt1 := frame.WithRegisterPublisher(topicRef2, "mem://topicB")

	ctx, srv := frame.NewService("Test Srv", opt, opt1, frame.WithNoopDriver())

	err := srv.Run(ctx, "")
	if err != nil {
		t.Errorf("we couldn't instantiate queue  %s", err)
		return
	}

	err = srv.Publish(ctx, topicRef, []byte("Testament"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %s", err)
		return
	}

	err = srv.Publish(ctx, topicRef2, []byte("Testament"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %s", err)
		return
	}

	err = srv.Publish(ctx, "test-multiple-3", []byte("Testament"))
	if err == nil {
		t.Errorf("We should not be able to publish to topic that was not registered")
		return
	}

	srv.Stop(ctx)
}

type msgHandler struct {
	f func(ctx context.Context, metadata map[string]string, message []byte) error
}

func (h *msgHandler) Handle(ctx context.Context, metadata map[string]string, message []byte) error {
	return h.f(ctx, metadata, message)
}

type handlerWithError struct {
}

func (m *handlerWithError) Handle(ctx context.Context, metadata map[string]string, message []byte) error {
	log := util.Log(ctx)
	log.Info("A dreadful message to handle", "message", string(message), "metadata", metadata)
	return errors.New("throwing an error for tests")
}

func TestService_RegisterSubscriber(t *testing.T) {
	regSubTopic := "test-reg-sub-topic"

	optTopic := frame.WithRegisterPublisher(regSubTopic, "mem://topicA")
	opt := frame.WithRegisterSubscriber(
		regSubTopic,
		"mem://topicA",
		&msgHandler{f: func(ctx context.Context, metadata map[string]string, message []byte) error {
			util.Log(ctx).WithField("metadata", metadata).WithField("message", string(message)).Info("Received message")
			return nil
		}},
	)

	ctx, srv := frame.NewService("Test Srv", optTopic, opt, frame.WithNoopDriver())

	err := srv.Run(ctx, ":")
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %s", err)
		return
	}

	for i := range make([]int, 30) {
		err = srv.Publish(ctx, regSubTopic, []byte(fmt.Sprintf(" testing message %d", i)))
		if err != nil {
			t.Errorf("We could not publish to a registered topic %d : %s ", i, err)
			return
		}
	}

	err = srv.Publish(ctx, regSubTopic, []byte("throw error"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %s", err)
		return
	}

	srv.Stop(ctx)
}

func TestService_RegisterSubscriberValidateMessages(t *testing.T) {
	regSubTopic := "test-reg-sub-pub-topic"

	var wg sync.WaitGroup
	receivedMessages := sync.Map{}

	handler := &msgHandler{f: func(ctx context.Context, metadata map[string]string, message []byte) error {
		msgStr := string(message)
		util.Log(ctx).WithField("metadata", metadata).WithField("message", msgStr).Info("Received message")
		receivedMessages.Store(msgStr, true)
		wg.Done() // Mark this message as processed
		return nil
	}}

	optTopic := frame.WithRegisterPublisher(regSubTopic, "mem://topicB")
	opt := frame.WithRegisterSubscriber(regSubTopic, "mem://topicB", handler)

	ctx, srv := frame.NewService("Test Srv", optTopic, opt, frame.WithNoopDriver())
	defer srv.Stop(ctx)

	err := srv.Run(ctx, ":")
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %s", err)
		return
	}

	emptyAny, err := json.Marshal(map[string]any{})
	if err != nil {
		t.Errorf("We couldn't marshal empty any")
		return
	}

	messages := []any{json.RawMessage("badjson"), emptyAny}
	expectedMsgs := map[string]bool{
		"badjson": false,
		"{}":      false,
	}

	for i := range 30 {
		msgStr := fmt.Sprintf("{\"id\": %d}", i)
		messages = append(messages, msgStr)
		expectedMsgs[msgStr] = false
	}

	wg.Add(len(messages))

	// Add a small delay between publishes to ensure all messages are properly committed to Jetstream
	for _, msg := range messages {
		err = srv.Publish(ctx, regSubTopic, msg)
		if err != nil {
			t.Errorf("We could not publish to a registered topic %v : %s ", msg, err)
			return
		}
		time.Sleep(time.Millisecond * 10) // Add small delay between publishes to ensure proper ordering
	}

	// Allow some time for JetStream to fully process all messages before checking
	time.Sleep(time.Millisecond * 500)

	// Wait for all messages with a timeout
	waitCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitCh)
	}()

	// Use a more generous timeout and check the actual received messages
	select {
	case <-time.After(time.Second * 10):
		// Check which messages were received and which are missing
		missingMsgs := []string{}
		for msg := range expectedMsgs {
			_, received := receivedMessages.Load(msg)
			if !received {
				missingMsgs = append(missingMsgs, msg)
			}
		}

		// Count received messages
		receivedCount := 0
		receivedMessages.Range(func(_, _ any) bool {
			receivedCount++
			return true
		})

		t.Errorf("We did not receive all %d messages, only %d on time. Missing: %v",
			len(messages), receivedCount, missingMsgs)
	case <-waitCh:
		// All messages received successfully
		t.Log("All messages received successfully")
	}
}

func TestService_SubscriberValidateJetstreamMessages(t *testing.T) {
	regSubTopic := "test-reg-sub-pub-topic"

	// Create unique identifiers for this test instance
	testID := strconv.FormatInt(time.Now().UnixNano(), 10)
	streamName := "frametest-" + testID
	subjectName := "frametest-" + testID
	durableName := "durableframe-" + testID

	receivedMessages := make(chan string, 1)
	defer close(receivedMessages)

	handler := &msgHandler{f: func(_ context.Context, _ map[string]string, message []byte) error {
		receivedMessages <- string(message)
		return nil
	}}

	// Configure JetStream for reliability:
	// 1. Explicit acknowledgment - ensures messages aren't removed until explicitly acknowledged
	// 2. Deliver policy "all" - ensures all messages are delivered
	// 3. Workqueue retention - ensures each message is sent to only one consumer in the group
	// 4. Memory storage - faster processing for tests
	// 5. Higher ack wait time - gives subscriber more time to process and acknowledge
	// 6. MaxAckPending matches message count - prevent flow control from limiting delivery
	streamOpt := fmt.Sprintf(
		"nats://frame:s3cr3t@localhost:4225?jetstream=true&subject=%s&stream_name=%s&stream_retention=workqueue&stream_storage=memory&stream_subjects=%s",
		subjectName,
		streamName,
		subjectName,
	)
	consumerOpt := fmt.Sprintf(
		"nats://frame:s3cr3t@localhost:4225?consumer_ack_policy=explicit&consumer_ack_wait=10s&consumer_deliver_policy=all&consumer_durable_name=%s&consumer_filter_subject=%s&jetstream=true&stream_name=%s&stream_retention=workqueue&stream_storage=memory&stream_subjects=%s&subject=%s",
		durableName,
		subjectName,
		streamName,
		subjectName,
		subjectName,
	)

	optTopic := frame.WithRegisterPublisher(regSubTopic, streamOpt)
	opt := frame.WithRegisterSubscriber(regSubTopic, consumerOpt, handler)

	ctx, srv := frame.NewService("Test Srv", optTopic, opt, frame.WithNoopDriver())
	defer srv.Stop(ctx)

	err := srv.Run(ctx, ":")
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %s", err)
		return
	}

	emptyAny, err := json.Marshal(map[string]any{})
	if err != nil {
		t.Errorf("We couldn't marshal empty any")
		return
	}

	messages := []any{json.RawMessage("badjson"), emptyAny}

	for i := range 30 {
		msgStr := fmt.Sprintf("{\"id\": %d}", i)
		messages = append(messages, msgStr)
	}

	// Add a longer delay between publishes to ensure proper JetStream commit
	for _, msg := range messages {
		err = srv.Publish(ctx, regSubTopic, msg)
		if err != nil {
			t.Errorf("We could not publish to a registered topic %v : %s ", msg, err)
			return
		}
	}

	// Track missing messages for logging/debugging
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	var received []string
	for {
		select {
		case v, ok := <-receivedMessages:

			if !ok {
				t.Errorf("We did not receive all %d messages, only %d on time. Missing: %v",
					len(messages),
					len(received),
					len(messages)-len(received),
				)
				return
			}

			received = append(received, v)

			if len(messages) == len(received) {
				t.Logf("All messages successfully handled")
				return
			}

		case <-ctx.Done():
			// Count final state of messages
			return
		case <-ticker.C:
			t.Errorf(
				"We did not receive all %d messages, only %d on time. Missing: %v",
				len(messages),
				len(received),
				len(messages)-len(received),
			)
			return
		}
	}
}

func TestService_RegisterSubscriberWithError(t *testing.T) {
	regSubT := "reg_s_wit-error"
	opt := frame.WithRegisterSubscriber(regSubT, "mem://topicErrors", &handlerWithError{})
	optTopic := frame.WithRegisterPublisher(regSubT, "mem://topicErrors")

	ctx, srv := frame.NewService("Test Srv", opt, optTopic, frame.WithNoopDriver())

	err := srv.Run(ctx, "")
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %s", err)
		return
	}

	err = srv.Publish(ctx, regSubT, []byte(" testing message with error"))
	if err != nil {
		t.Errorf("We could not publish to topic that was registered %s", err)
		return
	}

	srv.Stop(ctx)
}

func TestService_RegisterSubscriberInvalid(t *testing.T) {
	opt := frame.WithRegisterSubscriber("test", "memt+://topicA",
		&msgHandler{f: func(ctx context.Context, metadata map[string]string, message []byte) error {
			util.Log(ctx).WithField("metadata", metadata).WithField("message", string(message)).Info("Received message")
			return nil
		}})

	ctx, srv := frame.NewService("Test Srv", opt, frame.WithNoopDriver())

	defer srv.Stop(ctx)

	if err := srv.Run(ctx, ""); err == nil {
		t.Errorf("We somehow instantiated an invalid subscription ")
	}
}

func TestService_RegisterSubscriberContextCancelWorks(t *testing.T) {
	optTopic := frame.WithRegisterPublisher("test", "mem://topicA")
	opt := frame.WithRegisterSubscriber("test", "mem://topicA",
		&msgHandler{f: func(ctx context.Context, metadata map[string]string, message []byte) error {
			util.Log(ctx).WithField("metadata", metadata).WithField("message", string(message)).Info("Received message")
			return nil
		}})

	ctx, srv := frame.NewService("Test Srv", opt, optTopic, frame.WithNoopDriver())
	defer srv.Stop(ctx)

	subs, err := srv.GetSubscriber("test")
	if err != nil {
		t.Errorf("Could not get subscriber %s", err)
	}
	if subs == nil {
		t.Fatalf("Subscription is nil yet it should be defined")
	}
	if subs.Initiated() {
		t.Fatalf("Subscription is invalid yet it should be ok")
	}

	if err = srv.Run(ctx, ""); err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	if !subs.Initiated() {
		t.Fatalf("Subscription is valid yet it should not be ok")
	}
}

func TestService_AddPublisher(t *testing.T) {
	ctx, srv := frame.NewService("Test Srv", frame.WithNoopDriver())
	defer srv.Stop(ctx)

	if err := srv.Run(ctx, ""); err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	// Test case 1: Add a new publisher
	reference := "new-publisher"
	queueURL := "mem://topicX"

	err := srv.AddPublisher(ctx, reference, queueURL)
	if err != nil {
		t.Errorf("Failed to add a new publisher: %v", err)
	}

	// Verify the publisher was added
	pub, err := srv.GetPublisher(reference)
	if err != nil {
		t.Errorf("Failed to get publisher: %v", err)
	}
	if pub == nil {
		t.Error("Publisher was not added successfully")
	}

	// Test case 2: Add a publisher that already exists
	err = srv.AddPublisher(ctx, reference, queueURL)
	if err != nil {
		t.Errorf("Expected no error when adding an existing publisher, got: %v", err)
	}

	// Test case 3: Initialize and use a new publisher
	testPubRef := "test-pub-init"
	err = srv.AddPublisher(ctx, testPubRef, "mem://topicInit")
	if err != nil {
		t.Errorf("Failed to add and initialize publisher: %v", err)
	}

	// Run the service to ensure full initialization
	err = srv.Run(ctx, "")
	if err != nil {
		t.Errorf("Failed to run service: %v", err)
	}

	// Try to publish with the initialized publisher
	err = srv.Publish(ctx, testPubRef, []byte("test message"))
	if err != nil {
		t.Errorf("Failed to publish with initialized publisher: %v", err)
	}
}

func TestService_AddPublisher_InvalidURL(t *testing.T) {
	// Test with an invalid URL scheme that should cause an error
	ctx, srv := frame.NewService("Test Srv", frame.WithNoopDriver())
	defer srv.Stop(ctx)

	if err := srv.Run(ctx, ""); err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	// Attempt to add a publisher with an invalid URL scheme
	reference := "invalid-pub"
	queueURL := "invalid://topic" // This scheme is not registered

	// This should fail because the invalid URL can't be initialized
	err := srv.AddPublisher(ctx, reference, queueURL)
	if err == nil {
		t.Error("Expected error when adding publisher with invalid URL, but got nil")
	}
}

func TestService_AddSubscriber(t *testing.T) {
	ctx, srv := frame.NewService("Test Srv", frame.WithNoopDriver())
	defer srv.Stop(ctx)

	if err := srv.Run(ctx, ""); err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	// Test case 1: Add a new subscriber
	reference := "new-subscriber"
	queueURL := "mem://topicS"
	handler := &msgHandler{f: func(ctx context.Context, metadata map[string]string, message []byte) error {
		util.Log(ctx).WithField("metadata", metadata).WithField("message", string(message)).Info("Received message")
		return nil
	}}

	// First register a publisher to create the topic
	pubOpt := frame.WithRegisterPublisher(reference, queueURL)
	pubOpt(ctx, srv)

	// Run the service to initialize the publisher
	err := srv.Run(ctx, "")
	if err != nil {
		t.Errorf("Failed to run service: %v", err)
		return
	}

	// Now add the subscriber
	err = srv.AddSubscriber(ctx, reference, queueURL, handler)
	if err != nil {
		t.Errorf("Failed to add a new subscriber: %v", err)
	}

	// Verify the subscriber was added
	sub, err := srv.GetSubscriber(reference)
	if err != nil {
		t.Errorf("Failed to get subscriber: %v", err)
	}
	if sub == nil {
		t.Error("Subscriber was not added successfully")
	}

	// Test case 2: Add a subscriber that already exists
	err = srv.AddSubscriber(ctx, reference, queueURL, handler)
	if err != nil {
		t.Errorf("Expected no error when adding an existing subscriber, got: %v", err)
	}
}
func TestService_AddSubscriberWithoutHandler(t *testing.T) {
	noHandlerRef := "no-handlers-sub"
	noHandlerURL := "mem://topicNoHandler"

	optTopic := frame.WithRegisterPublisher(noHandlerRef, noHandlerURL)

	ctx, srv := frame.NewService("Test Srv 2", optTopic, frame.WithNoopDriver())
	defer srv.Stop(ctx)

	if err := srv.Run(ctx, ""); err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	// Run the service to initialize the publisher
	err := srv.Run(ctx, "")
	if err != nil {
		t.Errorf("Failed to run service: %v", err)
		return
	}

	// Now add the subscriber
	err = srv.AddSubscriber(ctx, noHandlerRef, noHandlerURL)
	if err != nil {
		t.Errorf("Failed to add subscriber without handlers: %v", err)
	}

	// Verify it was added
	sub, err := srv.GetSubscriber(noHandlerRef)
	if err != nil {
		t.Errorf("Failed to get subscriber: %v", err)
	}
	if sub == nil {
		t.Error("Subscriber without handlers was not added successfully")
	}

	// Clean up
}

func TestService_AddSubscriber_InvalidURL(t *testing.T) {
	// Test with an invalid URL scheme that should cause an error
	ctx, srv := frame.NewService("Test Srv", frame.WithNoopDriver())
	defer srv.Stop(ctx)

	if err := srv.Run(ctx, ""); err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	// Attempt to add a subscriber with an invalid URL scheme
	reference := "invalid-sub"
	queueURL := "invalid://topic" // This scheme is not registered
	handler := &msgHandler{f: func(ctx context.Context, metadata map[string]string, message []byte) error {
		util.Log(ctx).WithField("metadata", metadata).WithField("message", string(message)).Info("Received message")
		return nil
	}}

	// This should fail because the invalid URL can't be initialized
	err := srv.AddSubscriber(ctx, reference, queueURL, handler)
	if err == nil {
		t.Error("Expected error when adding subscriber with invalid URL, but got nil")
		srv.Stop(ctx) // Clean up if the test somehow passes
	}
}
