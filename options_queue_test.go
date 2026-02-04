package frame_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pitabwire/frame"
)

// msgHandler is a simple test handler.
type msgHandler struct {
	f func(context.Context, map[string]string, []byte) error
}

func (h *msgHandler) Handle(ctx context.Context, header map[string]string, message []byte) error {
	return h.f(ctx, header, message)
}

func TestOptionEarlyFailure(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		queueURL  string
		errMsg    string
		isSub     bool
	}{
		{
			name:      "WithRegisterPublisher empty reference reports error",
			reference: "",
			queueURL:  "mem://test",
			errMsg:    "publisher reference cannot be empty",
			isSub:     false,
		},
		{
			name:      "WithRegisterPublisher invalid queueURL reports error",
			reference: "test",
			queueURL:  "",
			errMsg:    "publisher queueURL is invalid",
			isSub:     false,
		},
		{
			name:      "WithRegisterSubscriber empty reference reports error",
			reference: "",
			queueURL:  "mem://test",
			errMsg:    "subscriber reference cannot be empty",
			isSub:     true,
		},
		{
			name:      "WithRegisterSubscriber invalid queueURL reports error",
			reference: "test",
			queueURL:  "",
			errMsg:    "subscriber queueURL is invalid",
			isSub:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opt frame.Option
			if tt.isSub {
				handler := &msgHandler{f: func(_ context.Context, _ map[string]string, _ []byte) error {
					return nil
				}}
				opt = frame.WithRegisterSubscriber(tt.reference, tt.queueURL, handler)
			} else {
				opt = frame.WithRegisterPublisher(tt.reference, tt.queueURL)
			}

			ctx, svc := frame.NewService(opt)
			defer svc.Stop(ctx)

			errs := svc.GetStartupErrors()
			if len(errs) == 0 {
				t.Fatalf("expected startup error containing '%s', but got none", tt.errMsg)
			}

			found := false
			for _, err := range errs {
				if err != nil && strings.Contains(err.Error(), tt.errMsg) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected startup error containing '%s', got %v", tt.errMsg, errs)
			}
		})
	}
}
