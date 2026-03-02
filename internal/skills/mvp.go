package skills

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// RegisterMVPSkills registers the Phase 1 MVP skills into the registry.
// bridgeURL is "http://127.0.0.1:8765" (the MT5 EA bridge).
// cronDeps is optional — if provided, registers the cron_add tool.
func RegisterMVPSkills(r *Registry, bridgeURL string, cronDeps *CronDeps) {
	r.Register(getPriceSkill())
	r.Register(placePendingSkill(bridgeURL))
	r.Register(cancelPendingSkill(bridgeURL))
	r.Register(getAccountInfoSkill())

	// Phase C tools
	if cronDeps != nil {
		r.Register(CronAddSkill(*cronDeps))
	}
	r.Register(webSearchSkill())
	r.Register(webFetchSkill())
}

// --- get_price ---

func getPriceSkill() *Skill {
	return &Skill{
		Name:        "get_price",
		Description: "Get current bid/ask price and spread for a trading symbol",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{
					"type":        "string",
					"description": "Trading symbol, e.g. XAUUSD, EURUSD",
				},
			},
			"required": []string{"symbol"},
		},
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Symbol string `json:"symbol"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("get_price: invalid args: %w", err)
			}
			// Stub — will be wired to bridge in production
			return fmt.Sprintf(`{"symbol":"%s","bid":0,"ask":0,"spread":0,"source":"stub"}`, p.Symbol), nil
		},
	}
}

// --- place_pending ---

type placePendingArgs struct {
	Type   string  `json:"type"` // BUY_LIMIT | SELL_LIMIT | BUY_STOP | SELL_STOP
	Symbol string  `json:"symbol"`
	Level  float64 `json:"level"`
	Lot    float64 `json:"lot"`
	SL     float64 `json:"sl"`
	TP     float64 `json:"tp"`
}

func placePendingSkill(bridgeURL string) *Skill {
	return &Skill{
		Name:        "place_pending",
		Description: "Place a pending order (LIMIT or STOP) in MT5 via EA bridge",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type":   map[string]any{"type": "string", "enum": []string{"BUY_LIMIT", "SELL_LIMIT", "BUY_STOP", "SELL_STOP"}},
				"symbol": map[string]any{"type": "string"},
				"level":  map[string]any{"type": "number", "description": "Entry price level"},
				"lot":    map[string]any{"type": "number"},
				"sl":     map[string]any{"type": "number", "description": "Stop loss price"},
				"tp":     map[string]any{"type": "number", "description": "Take profit price"},
			},
			"required": []string{"type", "symbol", "level", "lot", "sl", "tp"},
		},
		Execute: func(args json.RawMessage) (string, error) {
			var p placePendingArgs
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("place_pending: invalid args: %w", err)
			}

			payload, _ := json.Marshal(map[string]any{
				"action": "PLACE_PENDING",
				"type":   p.Type,
				"symbol": p.Symbol,
				"level":  p.Level,
				"lot":    p.Lot,
				"sl":     p.SL,
				"tp":     p.TP,
			})

			resp, err := http.Post(bridgeURL+"/signal", "application/json", bytes.NewReader(payload))
			if err != nil {
				return "", fmt.Errorf("place_pending: bridge error: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			return string(body), nil
		},
	}
}

// --- cancel_pending ---

func cancelPendingSkill(bridgeURL string) *Skill {
	return &Skill{
		Name:        "cancel_pending",
		Description: "Cancel an existing pending order by ticket number",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ticket": map[string]any{"type": "integer", "description": "MT5 order ticket number"},
			},
			"required": []string{"ticket"},
		},
		Execute: func(args json.RawMessage) (string, error) {
			var p struct {
				Ticket int64 `json:"ticket"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return "", fmt.Errorf("cancel_pending: invalid args: %w", err)
			}

			payload, _ := json.Marshal(map[string]any{
				"action": "CANCEL_PENDING",
				"ticket": p.Ticket,
			})

			resp, err := http.Post(bridgeURL+"/signal", "application/json", bytes.NewReader(payload))
			if err != nil {
				return "", fmt.Errorf("cancel_pending: bridge error: %w", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			return string(body), nil
		},
	}
}

// --- get_account_info ---

func getAccountInfoSkill() *Skill {
	return &Skill{
		Name:        "get_account_info",
		Description: "Get MT5 account balance, equity, and open positions",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Execute: func(args json.RawMessage) (string, error) {
			// Stub — will be wired to bridge in production
			return `{"balance":0,"equity":0,"open_positions":0,"source":"stub"}`, nil
		},
	}
}
