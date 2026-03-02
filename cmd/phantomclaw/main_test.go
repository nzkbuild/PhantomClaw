package main

import (
	"context"
	"testing"

	"go.uber.org/zap"
)

type fakeAlertSender struct {
	called bool
	text   string
}

func (f *fakeAlertSender) Send(ctx context.Context, text string) {
	f.called = true
	f.text = text
}

func TestMakeSessionAlertSenderCallsSend(t *testing.T) {
	logger := zap.NewNop().Sugar()
	sender := &fakeAlertSender{}

	cb := makeSessionAlertSender(sender, logger)
	cb(context.Background(), "hello alert")

	if !sender.called {
		t.Fatal("expected sender.Send to be called")
	}
	if sender.text != "hello alert" {
		t.Fatalf("sender text=%q, want=%q", sender.text, "hello alert")
	}
}

func TestMakeSessionAlertSenderHandlesNilSender(t *testing.T) {
	logger := zap.NewNop().Sugar()

	cb := makeSessionAlertSender(nil, logger)
	cb(context.Background(), "noop")
}
