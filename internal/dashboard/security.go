package dashboard

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// sensitiveKeys are field names that should be redacted before sending to browser.
var sensitiveKeys = []string{"key", "token", "secret", "password", "credential"}

// RedactMap deep-scans a map and replaces values whose keys contain
// sensitive substrings with "***". It modifies the map in-place.
func RedactMap(m map[string]any) map[string]any {
	if m == nil {
		return m
	}
	for k, v := range m {
		lower := strings.ToLower(k)
		if containsAny(lower, sensitiveKeys) {
			m[k] = "***"
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			RedactMap(val)
		case []any:
			for _, item := range val {
				if sub, ok := item.(map[string]any); ok {
					RedactMap(sub)
				}
			}
		}
	}
	return m
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// BasicAuth returns middleware that protects routes with HTTP Basic Auth.
// If user/pass are empty, the middleware is a no-op (no auth required).
func BasicAuth(user, pass string) func(http.Handler) http.Handler {
	if strings.TrimSpace(user) == "" || strings.TrimSpace(pass) == "" {
		return func(next http.Handler) http.Handler { return next }
	}

	// Pre-compute hash to avoid timing attacks on string comparison.
	userHash := sha256.Sum256([]byte(user))
	passHash := sha256.Sum256([]byte(pass))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, p, ok := r.BasicAuth()
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="PhantomClaw"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			uHash := sha256.Sum256([]byte(u))
			pHash := sha256.Sum256([]byte(p))
			if subtle.ConstantTimeCompare(uHash[:], userHash[:]) != 1 ||
				subtle.ConstantTimeCompare(pHash[:], passHash[:]) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="PhantomClaw"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
