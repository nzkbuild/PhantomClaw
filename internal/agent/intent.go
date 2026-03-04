package agent

import (
	"context"
	"strings"

	"github.com/nzkbuild/PhantomClaw/internal/llm"
)

// Intent represents the classified intent of a user chat message.
type Intent string

const (
	IntentTrading   Intent = "trading"   // P&L, positions, trades, market questions
	IntentCommand   Intent = "command"   // natural language commands ("switch to observe")
	IntentKnowledge Intent = "knowledge" // general trading/finance knowledge
)

// commandMapping maps natural language phrases to bot commands.
var commandMapping = map[string]string{
	"switch to observe": "/observe",
	"go to observe":     "/observe",
	"observe mode":      "/observe",
	"switch to suggest": "/suggest",
	"go to suggest":     "/suggest",
	"suggest mode":      "/suggest",
	"switch to auto":    "/auto",
	"go to auto":        "/auto",
	"auto mode":         "/auto",
	"halt":              "/halt",
	"stop trading":      "/halt",
	"emergency stop":    "/halt",
	"show status":       "/status",
	"what's the status": "/status",
	"show report":       "/report",
	"daily report":      "/report",
	"show pairs":        "/pairs",
	"active pairs":      "/pairs",
	"show confidence":   "/confidence",
	"confidence score":  "/confidence",
	"show config":       "/config",
	"help":              "/help",
	"show help":         "/help",
	"what can you do":   "/help",
	"show diagnostics":  "/diag",
	"run diagnostics":   "/diag",
	"show providers":    "/provider",
	"provider status":   "/provider",
	"show models":       "/model",
	"model status":      "/model",
}

// tradingKeywords trigger trading-context injection.
var tradingKeywords = []string{
	"pnl", "p&l", "profit", "loss", "position", "trade", "trades",
	"equity", "balance", "drawdown", "lot", "order", "pending",
	"xauusd", "eurusd", "usdjpy", "gbpusd", "gold", "forex",
	"signal", "entry", "exit", "stop loss", "take profit",
	"today's trades", "open positions", "daily loss",
	"how much", "how many trades", "win rate", "performance",
}

// ClassifyIntent determines the intent of a user message using keyword matching.
// Returns (intent, cmdHint). cmdHint is non-empty only for IntentCommand.
func ClassifyIntent(text string) (Intent, string) {
	lower := strings.ToLower(strings.TrimSpace(text))

	// Check for natural language commands first
	for phrase, cmd := range commandMapping {
		if strings.Contains(lower, phrase) {
			return IntentCommand, cmd
		}
	}

	// Check for trading-related queries
	for _, kw := range tradingKeywords {
		if strings.Contains(lower, kw) {
			return IntentTrading, ""
		}
	}

	// Default: general knowledge / conversation
	return IntentKnowledge, ""
}

// ClassifyIntentWithLLM uses the LLM as a fallback classifier when keyword
// matching returns IntentKnowledge. This catches edge cases like
// "how am I doing today" (should be trading) that keywords miss.
//
// The classifier runs a fast single-token classification prompt.
// Falls back to IntentKnowledge on any error.
func ClassifyIntentWithLLM(ctx context.Context, text string, chat func(ctx context.Context, msgs []llm.Message) (string, error)) Intent {
	classifyPrompt := `Classify this user message into exactly one category.
Reply with ONLY the category name, nothing else.

Categories:
- TRADING: questions about P&L, positions, trades, performance, market data
- COMMAND: requests to change bot mode, settings, or run bot commands
- KNOWLEDGE: general questions, explanations, learning, conversation

Message: "` + text + `"

Category:`

	result, err := chat(ctx, []llm.Message{
		{Role: "user", Content: classifyPrompt},
	})
	if err != nil {
		return IntentKnowledge
	}

	result = strings.ToLower(strings.TrimSpace(result))
	switch {
	case strings.Contains(result, "trading"):
		return IntentTrading
	case strings.Contains(result, "command"):
		return IntentCommand
	default:
		return IntentKnowledge
	}
}
