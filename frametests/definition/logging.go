package definition

import (
	"context"
	"time"

	"github.com/pitabwire/util"
	"github.com/testcontainers/testcontainers-go"
)

const DefaultLogProductionTimeout = 10 * time.Second

type StdoutLogConsumer struct {
	log *util.LogEntry
}

func LogConfig(ctx context.Context, timeout time.Duration) *testcontainers.LogConsumerConfig {
	return &testcontainers.LogConsumerConfig{
		Opts: []testcontainers.LogProductionOption{testcontainers.WithLogProductionTimeout(timeout)},
		Consumers: []testcontainers.LogConsumer{&StdoutLogConsumer{
			log: util.Log(ctx),
		}},
	}
}

// Accept prints the log to stdout.
func (s *StdoutLogConsumer) Accept(l testcontainers.Log) {
	if l.LogType == "STDOUT" {
		s.log.Info(string(l.Content))
	}
	if l.LogType == "STDERR" {
		s.log.Error(string(l.Content))
	}
}
