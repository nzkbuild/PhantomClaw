package llm

import (
	"errors"
	"strings"
)

// ErrorKind categorizes provider failures for smart routing decisions.
type ErrorKind int

const (
	// ErrRateLimit — 429: wait and retry the same provider.
	ErrRateLimit ErrorKind = iota
	// ErrAuth — 401/403: skip this provider, alert the owner.
	ErrAuth
	// ErrModelNotFound — 404: model doesn't exist on this provider, skip.
	ErrModelNotFound
	// ErrNetwork — timeout/connection refused: retry once, then fallback.
	ErrNetwork
	// ErrOverloaded — 503/529: provider is overloaded, fallback immediately.
	ErrOverloaded
	// ErrUnknown — anything else: fallback immediately.
	ErrUnknown
)

// String returns a human-readable label for the error kind.
func (k ErrorKind) String() string {
	switch k {
	case ErrRateLimit:
		return "rate_limit"
	case ErrAuth:
		return "auth_error"
	case ErrModelNotFound:
		return "model_not_found"
	case ErrNetwork:
		return "network_error"
	case ErrOverloaded:
		return "overloaded"
	default:
		return "unknown"
	}
}

// ProviderError wraps an error with classification metadata.
type ProviderError struct {
	Kind       ErrorKind
	ProviderID string
	Err        error
}

func (e *ProviderError) Error() string {
	return e.Err.Error()
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// ClassifyError inspects an error and returns a classified ProviderError.
// It checks for APIError (HTTP status codes) and falls back to string matching.
func ClassifyError(provider string, err error) *ProviderError {
	if err == nil {
		return nil
	}

	kind := ErrUnknown

	// Check for our typed APIError first (from generic.go rawPost)
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		kind = classifyStatusCode(apiErr.StatusCode)
		return &ProviderError{Kind: kind, ProviderID: provider, Err: err}
	}

	// Fall back to string matching for SDK errors (OpenAI, Claude)
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit"):
		kind = ErrRateLimit
	case strings.Contains(msg, "401") || strings.Contains(msg, "403") ||
		strings.Contains(msg, "unauthorized") || strings.Contains(msg, "forbidden") ||
		strings.Contains(msg, "invalid api key") || strings.Contains(msg, "authentication"):
		kind = ErrAuth
	case strings.Contains(msg, "404") || strings.Contains(msg, "model not found") ||
		strings.Contains(msg, "does not exist"):
		kind = ErrModelNotFound
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") || strings.Contains(msg, "network"):
		kind = ErrNetwork
	case strings.Contains(msg, "503") || strings.Contains(msg, "529") ||
		strings.Contains(msg, "overloaded") || strings.Contains(msg, "capacity"):
		kind = ErrOverloaded
	}

	return &ProviderError{Kind: kind, ProviderID: provider, Err: err}
}

func classifyStatusCode(code int) ErrorKind {
	switch {
	case code == 429:
		return ErrRateLimit
	case code == 401 || code == 403:
		return ErrAuth
	case code == 404:
		return ErrModelNotFound
	case code == 503 || code == 529:
		return ErrOverloaded
	case code >= 500:
		return ErrNetwork // treat server errors as retryable
	default:
		return ErrUnknown
	}
}
