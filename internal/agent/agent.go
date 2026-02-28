package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/bridge"
	"github.com/nzkbuild/PhantomClaw/internal/llm"
	"github.com/nzkbuild/PhantomClaw/internal/memory"
	"github.com/nzkbuild/PhantomClaw/internal/risk"
	"github.com/nzkbuild/PhantomClaw/internal/safety"
	"github.com/nzkbuild/PhantomClaw/internal/scheduler"
	"github.com/nzkbuild/PhantomClaw/internal/skills"
)

const (
	maxContextTokens = 2000 // PRD §13.7 — cap on injected context
	maxToolRounds    = 3    // Max ReAct iterations before forcing a decision
)

// Agent is the core intelligence — the ReAct brain that drives PhantomClaw.
type Agent struct {
	llm       llm.Provider
	skills    *skills.Registry
	memory    *memory.DB
	risk      *risk.Engine
	safety    *safety.Manager
	scheduler *scheduler.Scheduler
	pairs     []string
}

// New creates the agent brain.
func New(
	provider llm.Provider,
	skillReg *skills.Registry,
	db *memory.DB,
	riskEngine *risk.Engine,
	safetyMgr *safety.Manager,
	sched *scheduler.Scheduler,
	pairs []string,
) *Agent {
	return &Agent{
		llm:       provider,
		skills:    skillReg,
		memory:    db,
		risk:      riskEngine,
		safety:    safetyMgr,
		scheduler: sched,
		pairs:     pairs,
	}
}

// HandleSignal processes an EA signal through the ReAct loop and returns a trading decision.
// This is the core intelligence function wired to bridge POST /signal.
func (a *Agent) HandleSignal(ctx context.Context, req *bridge.SignalRequest) *bridge.SignalResponse {
	// Gate 1: Mode check
	if !a.safety.CanTrade() {
		return &bridge.SignalResponse{Action: "HOLD", Reason: "mode: " + a.safety.CurrentMode().String()}
	}

	// Gate 2: Weekend check
	if a.scheduler.IsWeekend() {
		return &bridge.SignalResponse{Action: "HOLD", Reason: "weekend — market closed"}
	}

	// Build context-injected prompt
	prompt := a.buildPrompt(req)

	// Build tool definitions for LLM
	tools := a.buildToolDefs()

	// ReAct loop: Think → Tool Call → Observe → Decide
	messages := []llm.Message{
		{Role: "system", Content: prompt.systemPrompt},
		{Role: "user", Content: prompt.userMessage},
	}

	var finalDecision string

	for round := 0; round < maxToolRounds; round++ {
		result, err := a.llm.ToolCall(ctx, messages, tools)
		if err != nil {
			log.Printf("agent: LLM error on round %d: %v", round, err)
			return &bridge.SignalResponse{Action: "HOLD", Reason: fmt.Sprintf("LLM error: %v", err)}
		}

		// If no tool calls → LLM has made its decision
		if len(result.ToolCalls) == 0 {
			finalDecision = result.Decision
			break
		}

		// Execute tool calls and feed results back
		for _, tc := range result.ToolCalls {
			toolResult, err := a.skills.Execute(tc.Name, json.RawMessage(tc.Arguments))
			if err != nil {
				toolResult = fmt.Sprintf(`{"error":"%s"}`, err.Error())
			}

			// Append assistant's tool call and tool result to conversation
			messages = append(messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("[Tool call: %s(%s)]", tc.Name, tc.Arguments),
			})
			messages = append(messages, llm.Message{
				Role:    "tool",
				Content: toolResult,
			})
		}
	}

	if finalDecision == "" {
		return &bridge.SignalResponse{Action: "HOLD", Reason: "agent: no decision after max rounds"}
	}

	// Parse LLM decision into a SignalResponse
	return a.parseDecision(finalDecision, req)
}

// promptContext holds the assembled prompt pieces.
type promptContext struct {
	systemPrompt string
	userMessage  string
}

// buildPrompt assembles the LLM prompt with context injection (PRD §13.7).
// Priority order: system identity → master strategy → session RAM → pair state → echo recall → signal data
func (a *Agent) buildPrompt(req *bridge.SignalRequest) promptContext {
	var sb strings.Builder

	// 1. System identity
	sb.WriteString("You are PhantomClaw, an autonomous forex/gold trading agent.\n")
	sb.WriteString("You analyze market data and decide whether to place pending orders.\n")
	sb.WriteString("You MUST respond with a JSON decision.\n\n")

	// 2. Current state
	sb.WriteString(fmt.Sprintf("Current session: %s\n", a.scheduler.CurrentSession()))
	sb.WriteString(fmt.Sprintf("Safety mode: %s\n", a.safety.CurrentMode()))

	riskStats := a.risk.Stats()
	sb.WriteString(fmt.Sprintf("Open positions: %d/%d\n", riskStats.OpenPositions, riskStats.MaxPositions))
	sb.WriteString(fmt.Sprintf("Daily loss: $%.2f\n", riskStats.DailyLoss))
	sb.WriteString(fmt.Sprintf("Profitable trades: %d/%d (ramp-up)\n\n", riskStats.ProfitableTrades, riskStats.RampUpTarget))

	// 3. Pair state (from memory)
	pairBias := a.loadPairContext(req.Symbol)
	if pairBias != "" {
		sb.WriteString("## Pair History\n")
		sb.WriteString(pairBias)
		sb.WriteString("\n\n")
	}

	// 4. Echo recall — past lessons for this symbol
	echoRecall := a.loadEchoRecall(req.Symbol)
	if echoRecall != "" {
		sb.WriteString("## Past Lessons (Echo Recall)\n")
		sb.WriteString(echoRecall)
		sb.WriteString("\n\n")
	}

	// 5. Decision format
	sb.WriteString("## Response Format\n")
	sb.WriteString("Respond with EXACTLY one JSON object:\n")
	sb.WriteString(`{"action":"PLACE_PENDING","type":"BUY_LIMIT|SELL_LIMIT|BUY_STOP|SELL_STOP","level":1.2345,"lot":0.05,"sl":1.2300,"tp":1.2400,"reason":"your reasoning"}`)
	sb.WriteString("\nOR\n")
	sb.WriteString(`{"action":"HOLD","reason":"why you are not trading"}`)
	sb.WriteString("\n\nUse tools to gather more data before deciding. Always check confidence score.\n")

	// Build user message with signal data
	signalJSON, _ := json.Marshal(req)

	return promptContext{
		systemPrompt: sb.String(),
		userMessage:  fmt.Sprintf("New signal received:\n```json\n%s\n```\nAnalyze this and decide: place a pending order or HOLD?", string(signalJSON)),
	}
}

// buildToolDefs converts registered skills to LLM tool format.
func (a *Agent) buildToolDefs() []llm.Tool {
	skillList := a.skills.List()
	tools := make([]llm.Tool, 0, len(skillList))
	for _, s := range skillList {
		params, _ := s["parameters"].(map[string]any)
		tools = append(tools, llm.Tool{
			Name:        s["name"].(string),
			Description: s["description"].(string),
			Parameters:  params,
		})
	}
	return tools
}

// loadPairContext retrieves pair state from memory for context injection.
func (a *Agent) loadPairContext(symbol string) string {
	// Query pair_state table
	var bias, prefTF string
	var winRate, avgPnl float64
	err := a.memory.QueryRow(
		"SELECT bias, preferred_tf, win_rate_7d, avg_pnl_7d FROM pair_state WHERE symbol = ?",
		symbol,
	).Scan(&bias, &prefTF, &winRate, &avgPnl)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("- %s: bias=%s, best_tf=%s, win_rate_7d=%.1f%%, avg_pnl=$%.2f",
		symbol, bias, prefTF, winRate*100, avgPnl)
}

// loadEchoRecall retrieves top lessons for this symbol.
func (a *Agent) loadEchoRecall(symbol string) string {
	lessons, err := a.memory.GetLessonsBySymbol(symbol, 5)
	if err != nil || len(lessons) == 0 {
		return ""
	}

	var sb strings.Builder
	for i, l := range lessons {
		sb.WriteString(fmt.Sprintf("%d. [w=%.1f] %s\n", i+1, l.Weight, l.Lesson))
		if i >= 4 {
			break
		}
	}
	return sb.String()
}

// parseDecision converts the LLM's text response into a bridge SignalResponse.
func (a *Agent) parseDecision(text string, req *bridge.SignalRequest) *bridge.SignalResponse {
	// Try to extract JSON from the response
	text = strings.TrimSpace(text)

	// Find JSON object in the text
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end < 0 || end <= start {
		return &bridge.SignalResponse{Action: "HOLD", Reason: "agent: could not parse LLM response"}
	}
	jsonStr := text[start : end+1]

	var decision struct {
		Action string  `json:"action"`
		Type   string  `json:"type"`
		Level  float64 `json:"level"`
		Lot    float64 `json:"lot"`
		SL     float64 `json:"sl"`
		TP     float64 `json:"tp"`
		Reason string  `json:"reason"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &decision); err != nil {
		return &bridge.SignalResponse{Action: "HOLD", Reason: fmt.Sprintf("agent: JSON parse error: %v", err)}
	}

	if decision.Action == "HOLD" {
		// Log the HOLD to diary
		a.memory.InsertDiary(
			time.Now().Format("2006-01-02"),
			"RESEARCH",
			fmt.Sprintf("HOLD on %s: %s", req.Symbol, decision.Reason),
		)
		return &bridge.SignalResponse{Action: "HOLD", Reason: decision.Reason}
	}

	// Run confidence scoring
	conf := skills.ScoreConfidence(skills.ConfidenceInput{
		Session: string(a.scheduler.CurrentSession()),
		// MTF bias and S/R will be populated when full analysis skills are wired
	})

	if conf.Action == "HARD_SKIP" || conf.Action == "SKIP" {
		a.memory.InsertDiary(
			time.Now().Format("2006-01-02"),
			"RESEARCH",
			fmt.Sprintf("Skipped %s %s: confidence=%d (%s)", decision.Type, req.Symbol, conf.Score, conf.Action),
		)
		return &bridge.SignalResponse{Action: "HOLD", Reason: fmt.Sprintf("confidence too low: %d/100 (%s)", conf.Score, conf.Action)}
	}

	// Adjust lot by confidence factor
	lot := decision.Lot * conf.LotFactor

	// Run risk engine check
	riskResult := a.risk.CheckTrade(risk.TradeProposal{
		Symbol:    req.Symbol,
		Direction: strings.Split(decision.Type, "_")[0], // BUY_LIMIT → BUY
		Lot:       lot,
		SL:        decision.SL,
		TP:        decision.TP,
		Entry:     decision.Level,
	})

	if !riskResult.Approved {
		return &bridge.SignalResponse{Action: "HOLD", Reason: "risk: " + riskResult.Reason}
	}

	// Record trade open in risk engine
	a.risk.RecordTradeOpen()

	// Log to diary
	a.memory.InsertDiary(
		time.Now().Format("2006-01-02"),
		"TRADE_OPEN",
		fmt.Sprintf("%s %s @ %.5f lot=%.2f SL=%.5f TP=%.5f confidence=%d reason=%s",
			decision.Type, req.Symbol, decision.Level, riskResult.AdjustedLot,
			decision.SL, decision.TP, conf.Score, decision.Reason),
	)

	return &bridge.SignalResponse{
		Action: "PLACE_PENDING",
		Type:   decision.Type,
		Symbol: req.Symbol,
		Level:  decision.Level,
		Lot:    riskResult.AdjustedLot,
		SL:     decision.SL,
		TP:     decision.TP,
		Reason: decision.Reason,
	}
}

// HandleTradeResult processes a closed trade — runs post-mortem and writes lessons.
func (a *Agent) HandleTradeResult(ctx context.Context, req *bridge.TradeResultRequest) {
	a.risk.RecordTradeClose(req.PnL)

	// Store trade in memory
	tradeID, err := a.memory.InsertTrade(&memory.Trade{
		Symbol:    req.Symbol,
		Direction: req.Direction,
		Entry:     req.Entry,
		Exit:      req.Exit,
		Lot:       req.Lot,
		PnL:       req.PnL,
		Session:   string(a.scheduler.CurrentSession()),
		OpenedAt:  time.Now(), // approximate
	})
	if err != nil {
		log.Printf("agent: failed to store trade: %v", err)
		return
	}

	// Diary entry
	outcome := "PROFIT"
	if req.PnL < 0 {
		outcome = "LOSS"
	}
	a.memory.InsertDiary(
		time.Now().Format("2006-01-02"),
		"TRADE_CLOSE",
		fmt.Sprintf("%s %s %s: PnL=$%.2f", outcome, req.Direction, req.Symbol, req.PnL),
	)

	// Async post-mortem via LLM
	go a.runPostMortem(ctx, tradeID, req)
}

// runPostMortem asks the LLM to analyze a closed trade and write a lesson.
func (a *Agent) runPostMortem(ctx context.Context, tradeID int64, req *bridge.TradeResultRequest) {
	prompt := fmt.Sprintf(
		"Analyze this closed trade and write a short lesson (1-2 sentences):\n"+
			"Symbol: %s, Direction: %s, Entry: %.5f, Exit: %.5f, PnL: $%.2f\n"+
			"What did we do right or wrong? What should we remember for next time?",
		req.Symbol, req.Direction, req.Entry, req.Exit, req.PnL,
	)

	lesson, err := a.llm.Chat(ctx, []llm.Message{
		{Role: "system", Content: "You are PhantomClaw's post-trade analyst. Write concise lessons from trade results."},
		{Role: "user", Content: prompt},
	})
	if err != nil {
		log.Printf("agent: post-mortem LLM error: %v", err)
		return
	}

	// Determine tags based on result
	tags := []string{}
	if req.PnL > 0 {
		tags = append(tags, "win")
	} else {
		tags = append(tags, "loss")
	}
	tagsJSON, _ := json.Marshal(tags)

	_, err = a.memory.InsertLesson(&memory.Lesson{
		TradeID: tradeID,
		Symbol:  req.Symbol,
		Session: string(a.scheduler.CurrentSession()),
		Lesson:  strings.TrimSpace(lesson),
		Tags:    string(tagsJSON),
		Weight:  1.0,
	})
	if err != nil {
		log.Printf("agent: failed to store lesson: %v", err)
	} else {
		log.Printf("agent: lesson stored for trade %d on %s", tradeID, req.Symbol)
	}
}
