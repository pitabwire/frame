package frame

import (
	"context"
	"errors"
	"fmt"
	"log"
	"testing"
	"time"
)

func TestService_RegisterPublisherNotSet(t *testing.T) {
	ctx := context.Background()

	srv := NewService( "Test Srv")

	err := srv.Publish(ctx, "random", []byte("") )

	if err == nil  {
		t.Errorf("We shouldn't be able to publish when no topic was registered")
	}

}



func TestService_RegisterPublisherNotInitialized(t *testing.T) {
	ctx := context.Background()
	opt := RegisterPublisher("test", "mem://topicA")
	srv := NewService( "Test Srv", opt)

	err := srv.Publish(ctx, "random", []byte("") )

	if err == nil  {
		t.Errorf("We shouldn't be able to publish when no topic was registered")
	}

}


func TestService_RegisterPublisher(t *testing.T) {
	ctx := context.Background()

	opt := RegisterPublisher("test", "mem://topicA")
	srv := NewService( "Test Srv", opt)

	err := srv.initPubsub(ctx)
	if err != nil  {
		t.Errorf("We couldn't instantiate queue  %v", err)
		return
	}

	err = srv.Publish(ctx, "test", []byte("") )
	if err != nil  {
		t.Errorf("We could not publish to topic that was registered %v", err)
	}

	srv.Stop()

}



func TestService_RegisterPublisherMultiple(t *testing.T) {
	ctx := context.Background()

	opt := RegisterPublisher("test", "mem://topicA")
	opt1 := RegisterPublisher("test-2", "mem://topicB")
	srv := NewService( "Test Srv", opt, opt1)

	err := srv.initPubsub(ctx)
	if err != nil  {
		t.Errorf("We couldn't instantiate queue  %v", err)
		return
	}

	err = srv.Publish(ctx, "test", []byte("Testament") )
	if err != nil  {
		t.Errorf("We could not publish to topic that was registered %v", err)
	}

	err = srv.Publish(ctx, "test-2", []byte("Testament") )
	if err != nil  {
		t.Errorf("We could not publish to topic that was registered %v", err)
	}

	err = srv.Publish(ctx, "test-3", []byte("Testament") )
	if err == nil  {
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
		5, &messageHandler{} )

	srv := NewService( "Test Srv", optTopic, opt)

	err := srv.initPubsub(ctx)
	if err != nil  {
		t.Errorf("We couldn't instantiate queue  %v", err)
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
	opt := RegisterSubscriber("test", "mem://topicA", 1, &handlerWithError{} )

	srv := NewService( "Test Srv", optTopic, opt)

	err := srv.initPubsub(ctx)
	if err != nil  {
		t.Errorf("We couldn't instantiate queue  %v", err)
		return
	}

		err = srv.Publish(ctx, "test", []byte(fmt.Sprintf(" testing message with error")))
		if err != nil {
			t.Errorf("We could not publish to topic that was registered %v", err)
		}
	srv.Stop()
}


func TestService_RegisterSubscriberInvalid(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opt := RegisterSubscriber("test", "memt+://topicA",
		5, &messageHandler{} )

	srv := NewService( "Test Srv", opt)

	err := srv.initPubsub(ctx)
	if err == nil  {
		t.Errorf("We somehow instantiated an invalid subscription ")
	}
}



func TestService_RegisterSubscriberContextCancelWorks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	optTopic := RegisterPublisher("test", "mem://topicA")
	opt := RegisterSubscriber("test", "mem://topicA",
		5, &messageHandler{} )

	srv := NewService( "Test Srv", opt, optTopic)

	err := srv.initPubsub(ctx)
	if err != nil  {
		t.Errorf("We somehow fail to instantiate subscription ")
	}


	if !srv.queue.subscriptionQueueMap["test"].isInit{
		t.Errorf("Subscription is invalid yet it should be ok")
	}

	cancel()
	time.Sleep(3 * time.Second)

	if srv.queue.subscriptionQueueMap["test"].isInit{
		t.Errorf("Subscription is valid yet it should not be ok")
	}


}