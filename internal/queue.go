package internal

import "context"

// AckID is the identifier of a message for purposes of acknowledgement.
type AckID any

// Message is data that moves around between client and server.
type Message struct {
	// ID message identifier for recorded messages on the server.
	ID string

	// Body contains the content of the message.
	Body []byte

	// Metadata has key/value pairs describing the message.
	Metadata map[string]string
}

// ByteSize estimates the size in bytes of the message for the purpose of restricting batch sizes.
func (m *Message) ByteSize() int {
	return len(m.Body)
}

type Queue interface {
	Send(ctx context.Context, ms ...*Message) error
	Receive(ctx context.Context, count int) ([]*Message, error)

	// Close cleans up any resources used by the Publisher.
	Close() error
}

type Subscriber interface {
	Handle(ctx context.Context, ms *Message) error

	// Close cleans up any resources used by the Subscriber.
	Close() error
}
