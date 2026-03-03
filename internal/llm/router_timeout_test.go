package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type timedProvider struct {
	name      string
	delay     time.Duration
	toolErr   error
	toolReply *ToolResult
	calls     int32
}

func (p *timedProvider) Name() string { return p.name }

func (p *timedProvider) Chat(ctx context.Context, _ []Message) (string, error) {
	select {
	case <-time.After(p.delay):
		if p.toolErr != nil {
			return "", p.toolErr
		}
		return "ok", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p *timedProvider) StreamChat(ctx context.Context, _ []Message, _ StreamCallback) error {
	select {
	case <-time.After(p.delay):
		return p.toolErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *timedProvider) ToolCall(ctx context.Context, _ []Message, _ []Tool) (*ToolResult, error) {
	atomic.AddInt32(&p.calls, 1)
	select {
	case <-time.After(p.delay):
		if p.toolErr != nil {
			return nil, p.toolErr
		}
		if p.toolReply != nil {
			return p.toolReply, nil
		}
		return &ToolResult{Decision: `{"action":"HOLD","reason":"ok"}`}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *timedProvider) callCount() int32 {
	return atomic.LoadInt32(&p.calls)
}

func TestRouterToolCallBudgetPerProvider(t *testing.T) {
	primarySlow := &timedProvider{
		name:    "primary",
		delay:   500 * time.Millisecond,
		toolErr: errors.New("upstream failed"),
	}
	fallbackFast := &timedProvider{
		name:      "fallback",
		delay:     50 * time.Millisecond,
		toolReply: &ToolResult{Decision: `{"action":"HOLD","reason":"fallback_ok"}`},
	}

	r := NewRouter(RouterConfig{
		Providers:      []Provider{primarySlow, fallbackFast},
		AttemptTimeout: 300 * time.Millisecond,
		StickyPrimary:  false,
	})

	// Shared parent deadline is intentionally tight. Router should still reserve
	// enough budget for fallback provider instead of letting primary consume all.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	got, err := r.ToolCall(ctx, []Message{{Role: "user", Content: "decide"}}, nil)
	if err != nil {
		t.Fatalf("ToolCall returned error: %v", err)
	}
	if got == nil || got.Decision == "" {
		t.Fatalf("ToolCall returned empty decision: %#v", got)
	}
	if got.Decision != `{"action":"HOLD","reason":"fallback_ok"}` {
		t.Fatalf("unexpected decision: %s", got.Decision)
	}
}

func TestRouterStickyPrimaryDisablesFallback(t *testing.T) {
	primary := &timedProvider{
		name:    "primary",
		delay:   10 * time.Millisecond,
		toolErr: errors.New("primary_failed"),
	}
	fallback := &timedProvider{
		name:      "fallback",
		delay:     10 * time.Millisecond,
		toolReply: &ToolResult{Decision: `{"action":"HOLD","reason":"fallback_ok"}`},
	}

	r := NewRouter(RouterConfig{
		Providers:      []Provider{primary, fallback},
		AttemptTimeout: 200 * time.Millisecond,
		StickyPrimary:  true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_, err := r.ToolCall(ctx, []Message{{Role: "user", Content: "decide"}}, nil)
	if err == nil {
		t.Fatal("expected primary error when sticky primary is enabled")
	}
	if primary.callCount() == 0 {
		t.Fatal("expected primary provider to be called")
	}
	if fallback.callCount() != 0 {
		t.Fatalf("expected fallback provider not to be called, got %d calls", fallback.callCount())
	}
}
