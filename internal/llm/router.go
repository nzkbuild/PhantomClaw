package llm

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Router selects a provider with error-aware fallback, per-provider cooldown,
// and model aliases. Inspired by OpenClaw's Execution Router pattern.
type Router struct {
	mu        sync.RWMutex
	providers []Provider           // ordered: primary first, then fallbacks
	aliases   map[string]string    // "fast" → provider name
	cooldowns map[string]time.Time // provider name → cooldown expiry
	failures  map[string]int       // provider name → consecutive failure count

	maxFailures    int           // consecutive failures before cooldown (default 3)
	cooldownPeriod time.Duration // how long a cooled-down provider waits (default 5 min)
}

// RouterConfig holds configuration for the smart router.
type RouterConfig struct {
	Providers      []Provider
	Aliases        map[string]string
	MaxFailures    int           // default: 3
	CooldownPeriod time.Duration // default: 5 min
}

// NewRouter creates a smart LLM router with error-aware fallback and cooldown.
func NewRouter(cfg RouterConfig) *Router {
	maxFail := cfg.MaxFailures
	if maxFail <= 0 {
		maxFail = 3
	}
	cooldown := cfg.CooldownPeriod
	if cooldown <= 0 {
		cooldown = 5 * time.Minute
	}
	aliases := cfg.Aliases
	if aliases == nil {
		aliases = make(map[string]string)
	}

	return &Router{
		providers:      cfg.Providers,
		aliases:        aliases,
		cooldowns:      make(map[string]time.Time),
		failures:       make(map[string]int),
		maxFailures:    maxFail,
		cooldownPeriod: cooldown,
	}
}

func (r *Router) Name() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.providers) > 0 {
		return "router:" + r.providers[0].Name()
	}
	return "router:none"
}

// Resolve maps an alias to a provider name. Returns the input if no alias exists.
func (r *Router) Resolve(alias string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name, ok := r.aliases[alias]; ok {
		return name
	}
	return alias
}

// Chat tries providers in order, skipping cooled-down ones, with error-aware fallback.
func (r *Router) Chat(ctx context.Context, messages []Message) (string, error) {
	r.mu.RLock()
	providers := r.providers
	r.mu.RUnlock()

	var lastErr error
	for _, p := range providers {
		if r.isCoolingDown(p.Name()) {
			log.Printf("llm/router: %s is cooling down, skipping", p.Name())
			continue
		}

		result, err := p.Chat(ctx, messages)
		if err == nil {
			r.recordSuccess(p.Name())
			return result, nil
		}

		lastErr = err
		r.handleFailure(p.Name(), err)
	}

	if lastErr == nil {
		return "", fmt.Errorf("llm/router: no providers available (all cooling down)")
	}
	return "", fmt.Errorf("llm/router: all providers failed, last: %w", lastErr)
}

// StreamChat tries providers in order with error-aware fallback.
func (r *Router) StreamChat(ctx context.Context, messages []Message, callback StreamCallback) error {
	r.mu.RLock()
	providers := r.providers
	r.mu.RUnlock()

	var lastErr error
	for _, p := range providers {
		if r.isCoolingDown(p.Name()) {
			continue
		}

		if err := p.StreamChat(ctx, messages, callback); err == nil {
			r.recordSuccess(p.Name())
			return nil
		} else {
			lastErr = err
			r.handleFailure(p.Name(), err)
		}
	}

	if lastErr == nil {
		return fmt.Errorf("llm/router: no providers available (all cooling down)")
	}
	return fmt.Errorf("llm/router: all providers failed for StreamChat, last: %w", lastErr)
}

// ToolCall tries providers in order with error-aware fallback.
func (r *Router) ToolCall(ctx context.Context, messages []Message, tools []Tool) (*ToolResult, error) {
	r.mu.RLock()
	providers := r.providers
	r.mu.RUnlock()

	var lastErr error
	for _, p := range providers {
		if r.isCoolingDown(p.Name()) {
			continue
		}

		result, err := p.ToolCall(ctx, messages, tools)
		if err == nil {
			r.recordSuccess(p.Name())
			return result, nil
		}

		lastErr = err
		r.handleFailure(p.Name(), err)
	}

	if lastErr == nil {
		return nil, fmt.Errorf("llm/router: no providers available (all cooling down)")
	}
	return nil, fmt.Errorf("llm/router: all providers failed for ToolCall, last: %w", lastErr)
}

// SetPrimary moves a provider to the front of the list at runtime.
func (r *Router) SetPrimary(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove from current position if exists
	for i, existing := range r.providers {
		if existing.Name() == p.Name() {
			r.providers = append(r.providers[:i], r.providers[i+1:]...)
			break
		}
	}
	// Prepend as primary
	r.providers = append([]Provider{p}, r.providers...)
}

// ProviderStatus returns the status of all registered providers.
func (r *Router) ProviderStatus() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := make(map[string]string)
	for _, p := range r.providers {
		name := p.Name()
		if r.isCoolingDownLocked(name) {
			remaining := time.Until(r.cooldowns[name]).Round(time.Second)
			status[name] = fmt.Sprintf("cooldown (%s remaining)", remaining)
		} else if count, ok := r.failures[name]; ok && count > 0 {
			status[name] = fmt.Sprintf("degraded (%d failures)", count)
		} else {
			status[name] = "healthy"
		}
	}
	return status
}

// --- Internal helpers ---

func (r *Router) handleFailure(provider string, err error) {
	classified := ClassifyError(provider, err)

	r.mu.Lock()
	defer r.mu.Unlock()

	r.failures[provider]++
	count := r.failures[provider]

	switch classified.Kind {
	case ErrRateLimit:
		log.Printf("llm/router: %s rate limited (failure %d/%d), will retry after pause",
			provider, count, r.maxFailures)
		// Rate limits get a short cooldown
		r.cooldowns[provider] = time.Now().Add(30 * time.Second)

	case ErrAuth:
		log.Printf("llm/router: %s auth error — disabling until restart: %v", provider, err)
		// Auth errors get permanent cooldown (until restart)
		r.cooldowns[provider] = time.Now().Add(24 * time.Hour)

	case ErrModelNotFound:
		log.Printf("llm/router: %s model not found — disabling: %v", provider, err)
		r.cooldowns[provider] = time.Now().Add(24 * time.Hour)

	case ErrOverloaded:
		log.Printf("llm/router: %s overloaded (failure %d/%d)", provider, count, r.maxFailures)
		r.cooldowns[provider] = time.Now().Add(60 * time.Second)

	default:
		log.Printf("llm/router: %s error (failure %d/%d): %v", provider, count, r.maxFailures, err)
		if count >= r.maxFailures {
			log.Printf("llm/router: %s exceeded %d failures, cooling down for %s",
				provider, r.maxFailures, r.cooldownPeriod)
			r.cooldowns[provider] = time.Now().Add(r.cooldownPeriod)
			r.failures[provider] = 0
		}
	}
}

func (r *Router) recordSuccess(provider string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failures[provider] = 0
	delete(r.cooldowns, provider)
}

func (r *Router) isCoolingDown(provider string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.isCoolingDownLocked(provider)
}

func (r *Router) isCoolingDownLocked(provider string) bool {
	expiry, ok := r.cooldowns[provider]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		// Cooldown expired — provider recovers automatically.
		// Keep this read-only because callers may hold RLock.
		return false
	}
	return true
}
