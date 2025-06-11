package frame_test

import (
	"context"
	"errors"
	"fmt"
	"github.com/pitabwire/frame"
	"log"
	"testing"
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

type messageHandler struct {
}

func (m *messageHandler) Handle(_ context.Context, metadata map[string]string, message []byte) error {
	log.Printf(" A nice message to handle: %v with headers [%v]", string(message), metadata)
	return nil
}

type handlerWithError struct {
}

func (m *handlerWithError) Handle(_ context.Context, metadata map[string]string, message []byte) error {
	log.Printf(" A dreadful message to handle: %v with headers [%v]", string(message), metadata)
	return errors.New("throwing an error for tests")

}

func TestService_RegisterSubscriber(t *testing.T) {
	regSubTopic := "test-reg-sub-topic"

	optTopic := frame.WithRegisterPublisher(regSubTopic, "mem://topicA")
	opt := frame.WithRegisterSubscriber(regSubTopic, "mem://topicA", &messageHandler{})

	ctx, srv := frame.NewService("Test Srv", optTopic, opt, frame.WithNoopDriver())

	err := srv.Run(ctx, ":")
	if err != nil {
		t.Errorf("We couldn't instantiate queue  %s", err)
		return
	}

	for i := range make([]int, 30) {
		err := srv.Publish(ctx, regSubTopic, []byte(fmt.Sprintf(" testing message %d", i)))
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
		&messageHandler{})

	ctx, srv := frame.NewService("Test Srv", opt, frame.WithNoopDriver())

	defer srv.Stop(ctx)

	if err := srv.Run(ctx, ""); err == nil {
		t.Errorf("We somehow instantiated an invalid subscription ")
	}
}

func TestService_RegisterSubscriberContextCancelWorks(t *testing.T) {

	optTopic := frame.WithRegisterPublisher("test", "mem://topicA")
	opt := frame.WithRegisterSubscriber("test", "mem://topicA",
		&messageHandler{})

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

	if err := srv.Run(ctx, ""); err != nil {
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
	handler := &messageHandler{}

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

	noHandlerRef := "no-handler-sub"
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
		t.Errorf("Failed to add subscriber without handler: %v", err)
	}

	// Verify it was added
	sub, err := srv.GetSubscriber(noHandlerRef)
	if err != nil {
		t.Errorf("Failed to get subscriber: %v", err)
	}
	if sub == nil {
		t.Error("Subscriber without handler was not added successfully")
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
	handler := &messageHandler{}

	// This should fail because the invalid URL can't be initialized
	err := srv.AddSubscriber(ctx, reference, queueURL, handler)
	if err == nil {
		t.Error("Expected error when adding subscriber with invalid URL, but got nil")
		srv.Stop(ctx) // Clean up if the test somehow passes
	}
}
