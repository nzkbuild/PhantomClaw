package bridge

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// WebhookAlert represents an inbound alert from external services (e.g., TradingView).
type WebhookAlert struct {
	Source   string `json:"source"`   // "tradingview", "economic_calendar", etc.
	Symbol   string `json:"symbol"`   // e.g. "XAUUSD"
	Action   string `json:"action"`   // "buy", "sell", "alert"
	Message  string `json:"message"`  // free-text alert body
	Priority string `json:"priority"` // "high", "normal", "low"
}

// WebhookHandler handles inbound webhook alerts from external services.
type WebhookHandler struct {
	authToken string
	onAlert   func(alert WebhookAlert)
}

// NewWebhookHandler creates a webhook handler with auth token validation.
func NewWebhookHandler(authToken string, onAlert func(WebhookAlert)) *WebhookHandler {
	return &WebhookHandler{
		authToken: authToken,
		onAlert:   onAlert,
	}
}

// ServeHTTP handles POST /api/webhook/alert requests.
func (wh *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth check
	if wh.authToken != "" {
		token := strings.TrimSpace(r.Header.Get("X-Phantom-Bridge-Token"))
		if token != wh.authToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Parse alert
	var alert WebhookAlert
	if err := json.NewDecoder(r.Body).Decode(&alert); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(alert.Source) == "" {
		alert.Source = "unknown"
	}
	if strings.TrimSpace(alert.Priority) == "" {
		alert.Priority = "normal"
	}

	log.Printf("webhook: alert received from=%s symbol=%s action=%s priority=%s",
		alert.Source, alert.Symbol, alert.Action, alert.Priority)

	if wh.onAlert != nil {
		wh.onAlert(alert)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "received",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
