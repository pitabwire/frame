package frame_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pitabwire/frame"
)

type fields struct {
	test    string
	counter int
}

func (f *fields) process(_ context.Context, _ frame.JobResultPipe) error {
	if f.test == "first error" {
		f.counter++
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
		runs    int
		wantErr bool
	}{
		{
			name:    "Happy path",
			fields:  fields{},
			runs:    1,
			wantErr: false,
		}, {
			name: "Happy path 2",
			fields: fields{
				test: "overriden",
			},
			runs:    1,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, srv := frame.NewService(tt.name,
				frame.WithNoopDriver(),
				frame.WithBackgroundConsumer(func(_ context.Context) error {
					return nil
				}))

			err := srv.Run(ctx, ":")
			if err != nil {
				t.Errorf("could not start a background consumer peacefully : %v", err)
			}

			job := frame.NewJob(tt.fields.process)

			if err = frame.SubmitJob(ctx, srv, job); (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}

			time.Sleep(50 * time.Millisecond)

			if tt.runs != job.Runs() {
				t.Errorf("Test error could not retry for some reason, expected %d runs got %d ", tt.runs, job.Runs())
			}
		})
	}
}

func TestService_NewJobWithRetry(t *testing.T) {
	tests := []struct {
		name    string
		fields  fields
		runs    int
		wantErr bool
	}{
		{
			name: "Happy path",
			fields: fields{
				test: "first error",
			},
			runs:    2,
			wantErr: false,
		}, {
			name: "Happy path no error",
			fields: fields{
				test: "first error",
			},
			runs:    2,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, srv := frame.NewService(tt.name,
				frame.WithNoopDriver(),
				frame.WithBackgroundConsumer(func(_ context.Context) error {
					return nil
				}))

			err := srv.Run(ctx, ":")
			if err != nil {
				t.Errorf("could not start a background consumer peacefully : %v", err)
			}

			job := frame.NewJobWithRetry(tt.fields.process, 1)

			if err = frame.SubmitJob(ctx, srv, job); (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}

			time.Sleep(50 * time.Millisecond)

			if tt.runs != job.Runs() {
				t.Errorf("Test error could not retry for some reason")
			}
		})
	}
}

func TestService_NewJobWithBufferAndRetry(t *testing.T) {
	tests := []struct {
		name    string
		fields  fields
		runs    int
		wantErr bool
	}{
		{
			name: "Happy path",
			fields: fields{
				test: "first error",
			},
			runs:    2,
			wantErr: false,
		}, {
			name: "Happy path no error",
			fields: fields{
				test: "first error",
			},
			runs:    2,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, srv := frame.NewService(tt.name,
				frame.WithNoopDriver(),
				frame.WithBackgroundConsumer(func(_ context.Context) error {
					return nil
				}))

			err := srv.Run(ctx, ":")
			if err != nil {
				t.Errorf("could not start a background consumer peacefully : %v", err)
			}

			job := frame.NewJobWithBufferAndRetry(tt.fields.process, 4, 1)

			err = frame.SubmitJob(ctx, srv, job)
			if (err != nil) != tt.wantErr {
				t.Errorf("Process() error = %v, wantErr %v", err, tt.wantErr)
			}

			select {
			case _, ok := <-job.ResultChan():
				if !ok {
					t.Logf("result chan closed")
				}
			case <-time.Tick(500 * time.Millisecond):
				t.Fatalf("could not handle job within timelimit")
			}

			if tt.runs != job.Runs() {
				t.Errorf("Test retry count is not consistent: expected : %d got : %d ", tt.runs, job.Runs())
			}
		})
	}
}
