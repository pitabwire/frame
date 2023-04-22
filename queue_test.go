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
	opt := frame.RegisterPublisher("test", "mem://topicA")
	ctx, srv := frame.NewService("Test Srv", opt)

	err := srv.Publish(ctx, "random", []byte(""))

	if err == nil {
		t.Errorf("We shouldn't be able to publish when no topic was registered")
	}

}

func TestService_RegisterPublisher(t *testing.T) {

	opt := frame.RegisterPublisher("test", "mem://topicA")

	ctx, srv := frame.NewService("Test Srv", opt, frame.NoopDriver())

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

	opt := frame.RegisterPublisher(topicRef, "mem://topicA")
	opt1 := frame.RegisterPublisher(topicRef2, "mem://topicB")

	ctx, srv := frame.NewService("Test Srv", opt, opt1, frame.NoopDriver())

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
	regSubTopic := "test-reg-sub-topic"

	optTopic := frame.RegisterPublisher(regSubTopic, "mem://topicA")
	opt := frame.RegisterSubscriber(regSubTopic, "mem://topicA", 5, &messageHandler{})

	ctx, srv := frame.NewService("Test Srv", optTopic, opt, frame.NoopDriver())

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
	opt := frame.RegisterSubscriber(regSubT, "mem://topicErrors", 1, &handlerWithError{})
	optTopic := frame.RegisterPublisher(regSubT, "mem://topicErrors")

	ctx, srv := frame.NewService("Test Srv", opt, optTopic, frame.NoopDriver())

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

	opt := frame.RegisterSubscriber("test", "memt+://topicA",
		5, &messageHandler{})

	ctx, srv := frame.NewService("Test Srv", opt, frame.NoopDriver())

	defer srv.Stop(ctx)

	if err := srv.Run(ctx, ""); err == nil {
		t.Errorf("We somehow instantiated an invalid subscription ")
	}
}

func TestService_RegisterSubscriberContextCancelWorks(t *testing.T) {

	optTopic := frame.RegisterPublisher("test", "mem://topicA")
	opt := frame.RegisterSubscriber("test", "mem://topicA",
		5, &messageHandler{})

	ctx, srv := frame.NewService("Test Srv", opt, optTopic, frame.NoopDriver())

	if srv.SubscriptionIsInitiated("test") {
		t.Errorf("Subscription is invalid yet it should be ok")
	}

	if err := srv.Run(ctx, ""); err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	if !srv.SubscriptionIsInitiated("test") {
		t.Errorf("Subscription is valid yet it should not be ok")
	}

	srv.Stop(ctx)

}
