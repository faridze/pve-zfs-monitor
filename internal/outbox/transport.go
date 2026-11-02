package outbox

import "context"

// Transport delivers formatted messages. Implementations must be explicit opt-in.
type Transport interface {
	Send(ctx context.Context, msg Message) error
}

// FakeTransport records messages for synthetic tests (no network).
type FakeTransport struct {
	Sent []Message
	Err  error
}

func (f *FakeTransport) Send(ctx context.Context, msg Message) error {
	if f.Err != nil {
		return f.Err
	}
	f.Sent = append(f.Sent, msg)
	return nil
}
