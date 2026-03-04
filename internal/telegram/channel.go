package telegram

import "context"

// Channel defines the interface for message delivery channels.
// Telegram is the current implementation. Adding Discord, Slack, or others
// requires implementing this interface — agent code stays unchanged.
type Channel interface {
	// Send sends a message to the configured chat/channel.
	Send(ctx context.Context, text string)

	// Start begins listening for inbound messages (blocking).
	Start(ctx context.Context)
}

// Verify that Bot implements Channel at compile time.
var _ Channel = (*Bot)(nil)
