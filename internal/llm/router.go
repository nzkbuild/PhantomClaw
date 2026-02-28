package llm

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Router selects a provider from registered backends with automatic fallback.
// Primary provider is tried first; on failure, falls back to alternates in order.
type Router struct {
	mu       sync.RWMutex
	primary  Provider
	fallback []Provider
}

// NewRouter creates an LLM router with a primary provider and optional fallbacks.
func NewRouter(primary Provider, fallbacks ...Provider) *Router {
	return &Router{
		primary:  primary,
		fallback: fallbacks,
	}
}

func (r *Router) Name() string {
	return "router:" + r.primary.Name()
}

// Chat tries the primary provider, falling back on error.
func (r *Router) Chat(ctx context.Context, messages []Message) (string, error) {
	r.mu.RLock()
	primary := r.primary
	fallbacks := r.fallback
	r.mu.RUnlock()

	result, err := primary.Chat(ctx, messages)
	if err == nil {
		return result, nil
	}
	log.Printf("llm/router: %s failed: %v, trying fallbacks", primary.Name(), err)

	for _, fb := range fallbacks {
		result, err = fb.Chat(ctx, messages)
		if err == nil {
			log.Printf("llm/router: succeeded with fallback %s", fb.Name())
			return result, nil
		}
		log.Printf("llm/router: fallback %s failed: %v", fb.Name(), err)
	}

	return "", fmt.Errorf("llm/router: all providers failed, last error: %w", err)
}

// StreamChat tries the primary provider, falling back on error.
func (r *Router) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) error {
	r.mu.RLock()
	primary := r.primary
	fallbacks := r.fallback
	r.mu.RUnlock()

	if err := primary.StreamChat(ctx, messages, callback); err == nil {
		return nil
	}

	for _, fb := range fallbacks {
		if err := fb.StreamChat(ctx, messages, callback); err == nil {
			return nil
		}
	}

	return fmt.Errorf("llm/router: all providers failed for StreamChat")
}

// ToolCall tries the primary provider, falling back on error.
func (r *Router) ToolCall(ctx context.Context, messages []Message, tools []Tool) (*ToolResult, error) {
	r.mu.RLock()
	primary := r.primary
	fallbacks := r.fallback
	r.mu.RUnlock()

	result, err := primary.ToolCall(ctx, messages, tools)
	if err == nil {
		return result, nil
	}
	log.Printf("llm/router: %s ToolCall failed: %v, trying fallbacks", primary.Name(), err)

	for _, fb := range fallbacks {
		result, err = fb.ToolCall(ctx, messages, tools)
		if err == nil {
			return result, nil
		}
	}

	return nil, fmt.Errorf("llm/router: all providers failed for ToolCall, last: %w", err)
}

// SetPrimary swaps the primary provider at runtime.
func (r *Router) SetPrimary(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.primary = p
}
