package frame_test

import (
	"context"
	"errors"
	"github.com/pitabwire/frame"
	"testing"
	"time"
)

type fields struct {
	sleepTime time.Duration
	test      string
	counter   int
}

func (f *fields) process(ctx context.Context) error {

	if f.test == "first error" {
		f.counter += 1
		f.test = "erred"
		return errors.New("test error")
	}

	f.test = "confirmed"
	return nil
}

func TestJobImpl_Process(t *testing.T) {

	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Happy path",
			fields: fields{
				sleepTime: 1 * time.Second,
			},
			wantErr: false,
		}, {
			name: "Happy path 2",
			fields: fields{
				sleepTime: 1 * time.Second,
				test:      "overriden",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctx, srv := frame.NewService(tt.name,
				frame.NoopDriver(),
				frame.BackGroundConsumer(func(ctx context.Context) error {
					return nil
				}))

			err := srv.Run(ctx, ":")
			if err != nil {
				t.Errorf("could not start a background consumer peacefully : %v", err)
			}

			job := srv.NewJob(tt.fields.process)

			if err := srv.SubmitJob(ctx, job); (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}

			time.Sleep(50 * time.Millisecond)

			if "confirmed" != tt.fields.test {
				t.Errorf("Test error could not confirm function run")
			}

		})
	}
}

func TestService_NewJobWithRetry(t *testing.T) {

	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Happy path",
			fields: fields{
				sleepTime: 1 * time.Second,
				test:      "first error",
			},
			wantErr: false,
		}, {
			name: "Happy path no error",
			fields: fields{
				sleepTime: 1 * time.Second,
				test:      "first error",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			ctx, srv := frame.NewService(tt.name,
				frame.NoopDriver(),
				frame.BackGroundConsumer(func(ctx context.Context) error {
					return nil
				}))

			err := srv.Run(ctx, ":")
			if err != nil {
				t.Errorf("could not start a background consumer peacefully : %v", err)
			}

			job := srv.NewJobWithRetry(tt.fields.process, 1)

			if err := srv.SubmitJob(ctx, job); (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}

			time.Sleep(50 * time.Millisecond)

			if tt.fields.counter == 0 {
				t.Errorf("Test error could not retry for some reason")
			}

			if "confirmed" != tt.fields.test {
				t.Errorf("Test error could not confirm function run")
			}

		})
	}
}

func TestService_NewJobWithRetryAndErrorChan(t *testing.T) {

	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Happy path",
			fields: fields{
				sleepTime: 1 * time.Second,
				test:      "first error",
			},
			wantErr: false,
		}, {
			name: "Happy path no error",
			fields: fields{
				sleepTime: 1 * time.Second,
				test:      "first error",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			errChan := make(chan error, 1)
			defer close(errChan)

			ctx, srv := frame.NewService(tt.name,
				frame.NoopDriver(),
				frame.BackGroundConsumer(func(ctx context.Context) error {
					return nil
				}))

			err := srv.Run(ctx, ":")
			if err != nil {
				t.Errorf("could not start a background consumer peacefully : %v", err)
			}

			job := srv.NewJobWithRetryAndErrorChan(tt.fields.process, 1, errChan)

			if err := srv.SubmitJob(ctx, job); (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}

			select {
			case <-errChan:
				break
			case <-time.Tick(500 * time.Millisecond):
				t.Errorf("could not handle job within timelimit")
				break
			}

			if "confirmed" != tt.fields.test {
				t.Errorf("Test error could not confirm function run")
			}

		})
	}
}
