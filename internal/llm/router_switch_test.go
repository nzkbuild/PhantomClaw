package llm

import (
	"context"
	"sync"
	"testing"
)

// stubProvider is a minimal Provider for switch-guard tests.
type stubProvider struct{ name string }

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) Chat(_ context.Context, _ []Message) (string, error) {
	return "ok", nil
}
func (s *stubProvider) StreamChat(_ context.Context, _ []Message, _ StreamCallback) error {
	return nil
}
func (s *stubProvider) ToolCall(_ context.Context, _ []Message, _ []Tool) (*ToolResult, error) {
	return &ToolResult{}, nil
}

func newTestRouter(providers ...Provider) *Router {
	return NewRouter(RouterConfig{Providers: providers})
}

// TestSetPrimaryQueued_NoSignal verifies immediate apply when no signal is in flight.
func TestSetPrimaryQueued_NoSignal(t *testing.T) {
	a := &stubProvider{"alpha"}
	b := &stubProvider{"beta"}
	r := newTestRouter(a, b)

	applied := r.SetPrimaryQueued(b)
	if !applied {
		t.Fatal("expected immediate apply when no signal in flight")
	}
	if got := r.providers[0].Name(); got != "beta" {
		t.Fatalf("primary = %q, want %q", got, "beta")
	}
}

// TestSetPrimaryQueued_DuringSignal verifies the switch is deferred until EndSignal.
func TestSetPrimaryQueued_DuringSignal(t *testing.T) {
	a := &stubProvider{"alpha"}
	b := &stubProvider{"beta"}
	r := newTestRouter(a, b)

	r.BeginSignal()

	applied := r.SetPrimaryQueued(b)
	if applied {
		t.Fatal("expected queued (not immediate) while signal in flight")
	}
	// Primary should still be alpha during the signal.
	if got := r.providers[0].Name(); got != "alpha" {
		t.Fatalf("primary during signal = %q, want %q", got, "alpha")
	}

	r.EndSignal()

	// After EndSignal the queued switch must have been applied.
	if got := r.providers[0].Name(); got != "beta" {
		t.Fatalf("primary after EndSignal = %q, want %q", got, "beta")
	}
}

// TestSetPrimaryQueued_ConcurrentSignals verifies the switch waits for all signals.
func TestSetPrimaryQueued_ConcurrentSignals(t *testing.T) {
	a := &stubProvider{"alpha"}
	b := &stubProvider{"beta"}
	r := newTestRouter(a, b)

	r.BeginSignal()
	r.BeginSignal() // two concurrent signals

	r.SetPrimaryQueued(b)

	r.EndSignal() // first ends — switch must NOT have applied yet
	if got := r.providers[0].Name(); got != "alpha" {
		t.Fatalf("primary after first EndSignal = %q, want %q (should still be alpha)", got, "alpha")
	}

	r.EndSignal() // second ends — switch must now apply
	if got := r.providers[0].Name(); got != "beta" {
		t.Fatalf("primary after last EndSignal = %q, want %q", got, "beta")
	}
}

// TestSetPrimaryQueued_LastWriteWins verifies only the latest queued switch is applied.
func TestSetPrimaryQueued_LastWriteWins(t *testing.T) {
	a := &stubProvider{"alpha"}
	b := &stubProvider{"beta"}
	c := &stubProvider{"gamma"}
	r := newTestRouter(a, b, c)

	r.BeginSignal()
	r.SetPrimaryQueued(b)
	r.SetPrimaryQueued(c) // overrides b

	r.EndSignal()

	if got := r.providers[0].Name(); got != "gamma" {
		t.Fatalf("primary = %q, want %q (last write wins)", got, "gamma")
	}
}

// TestBeginEndSignal_NegativeGuard verifies EndSignal never goes negative.
func TestBeginEndSignal_NegativeGuard(t *testing.T) {
	a := &stubProvider{"alpha"}
	b := &stubProvider{"beta"}
	r := newTestRouter(a, b)

	r.EndSignal() // mismatched — should not panic or go negative
	applied := r.SetPrimaryQueued(b)
	if !applied {
		t.Fatal("expected immediate apply after mismatched EndSignal guard")
	}
	if got := r.providers[0].Name(); got != "beta" {
		t.Fatalf("primary=%q, want=%q", got, "beta")
	}
}

// TestSetPrimaryQueued_Race is a data-race sanity check for the concurrent path.
func TestSetPrimaryQueued_Race(t *testing.T) {
	a := &stubProvider{"alpha"}
	b := &stubProvider{"beta"}
	r := newTestRouter(a, b)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.BeginSignal()
			if n%2 == 0 {
				r.SetPrimaryQueued(b)
			}
			r.EndSignal()
		}(i)
	}
	wg.Wait()
}

func TestProviderByName_CaseInsensitive(t *testing.T) {
	a := &stubProvider{"Alpha"}
	b := &stubProvider{"beta"}
	r := newTestRouter(a, b)

	got := r.ProviderByName("ALPHA")
	if got == nil || got.Name() != "Alpha" {
		t.Fatalf("ProviderByName(ALPHA) = %v, want Alpha", got)
	}
	if miss := r.ProviderByName("does-not-exist"); miss != nil {
		t.Fatalf("ProviderByName for missing provider should return nil, got %v", miss)
	}
}

func TestProviderStatus_UnknownUntilFirstAttempt(t *testing.T) {
	a := &stubProvider{"alpha"}
	r := newTestRouter(a)

	status := r.ProviderStatus()
	if status["alpha"] != "unknown" {
		t.Fatalf("initial status=%q, want unknown", status["alpha"])
	}

	_, err := r.Chat(context.Background(), []Message{{Role: "user", Content: "ping"}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	status = r.ProviderStatus()
	if status["alpha"] != "healthy" {
		t.Fatalf("status after success=%q, want healthy", status["alpha"])
	}
}
