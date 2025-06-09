package frame_test

import (
	"context"
	"fmt"
	"github.com/pitabwire/frame"
	"testing"
	"time"
)

type MessageToTest struct {
	Service *frame.Service
	Count   int
}

func (event *MessageToTest) Name() string {
	return "message.to.test"
}

func (event *MessageToTest) PayloadType() any {
	pType := ""
	return &pType
}

func (event *MessageToTest) Validate(ctx context.Context, payload any) error {
	if _, ok := payload.(*string); !ok {
		return fmt.Errorf(" payload is %T not of type %T", payload, event.PayloadType())
	}

	return nil
}

func (event *MessageToTest) Execute(ctx context.Context, payload any) error {
	message := payload.(*string)
	logger := event.Service.L(ctx).WithField("payload", message).WithField("type", event.Name())
	logger.Info("handling event")
	event.Count = event.Count + 1
	return nil
}

func TestService_RegisterEventsWorks(t *testing.T) {
	cfg, err := frame.ConfigFromEnv[frame.ConfigurationDefault]()
	if err != nil {
		t.Errorf("could not processFunc configs %s", err)
		return
	}
	events := frame.RegisterEvents(&MessageToTest{})

	ctx, srv := frame.NewService("Test Srv", events, frame.Config(&cfg), frame.NoopDriver())

	subs, _ := srv.GetSubscriber(cfg.EventsQueueName)
	if subs != nil && subs.Initiated() {
		t.Fatalf("Subscription to event queue is invalid")
	}

	err = srv.Run(ctx, "")
	if err != nil {
		t.Errorf("We somehow fail to instantiate subscription ")
	}

	subs, _ = srv.GetSubscriber(cfg.EventsQueueName)
	if !subs.Initiated() {
		t.Fatalf("Subscription to event queue is not done, should be subscribed")
	}

	srv.Stop(ctx)

}

func TestService_EventsPublishingWorks(t *testing.T) {
	cfg, err := frame.ConfigFromEnv[frame.ConfigurationDefault]()
	if err != nil {
		t.Errorf("could not processFunc configs %s", err)
		return
	}
	testEvent := MessageToTest{Count: 50}
	events := frame.RegisterEvents(&testEvent)

	ctx, srv := frame.NewService("Test Srv", frame.Config(&cfg), frame.NoopDriver())
	srv.Init(events)
	if err = srv.Run(ctx, ""); err != nil {
		t.Errorf("We somehow fail to instantiate subscription %s", err)
	}

	err = srv.Emit(ctx, testEvent.Name(), "££ yoow")
	if err != nil {
		t.Errorf("We failed to emit a job %s", err)
	}
	time.Sleep(2 * time.Second)
	if testEvent.Count != 51 {
		t.Errorf("Subscription event was not processed")
	}

	srv.Stop(ctx)

}
