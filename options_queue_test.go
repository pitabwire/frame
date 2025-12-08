package frame

import (
	"context"
	"strings"
	"testing"
)

// msgHandler is a simple test handler
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
		panicMsg  string
	}{
		{
			name:      "WithRegisterPublisher empty reference panics",
			reference: "",
			queueURL:  "mem://test",
			panicMsg:  "publisher reference cannot be empty",
		},
		{
			name:      "WithRegisterPublisher empty queueURL panics",
			reference: "test",
			queueURL:  "",
			panicMsg:  "publisher queueURL cannot be empty",
		},
		{
			name:      "WithRegisterSubscriber empty reference panics",
			reference: "",
			queueURL:  "mem://test",
			panicMsg:  "subscriber reference cannot be empty",
		},
		{
			name:      "WithRegisterSubscriber empty queueURL panics",
			reference: "test",
			queueURL:  "",
			panicMsg:  "subscriber queueURL cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if r != tt.panicMsg {
						t.Errorf("expected panic message '%s', got '%v'", tt.panicMsg, r)
					}
				} else {
					t.Errorf("expected panic with message '%s', but no panic occurred", tt.panicMsg)
				}
			}()

			if strings.Contains(tt.name, "Publisher") {
				_ = WithRegisterPublisher(tt.reference, tt.queueURL)
			} else {
				handler := &msgHandler{f: func(_ context.Context, _ map[string]string, _ []byte) error {
					return nil
				}}
				_ = WithRegisterSubscriber(tt.reference, tt.queueURL, handler)
			}
		})
	}
}
