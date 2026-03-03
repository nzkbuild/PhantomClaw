package telegram

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
	"github.com/nzkbuild/PhantomClaw/internal/risk"
	"github.com/nzkbuild/PhantomClaw/internal/safety"
	"github.com/nzkbuild/PhantomClaw/internal/scheduler"
	"github.com/nzkbuild/PhantomClaw/internal/skills"
)

// Dependencies holds references to subsystems for command handlers.
type Dependencies struct {
	Safety      *safety.Manager
	Risk        *risk.Engine
	Scheduler   *scheduler.Scheduler
	Memory      *memory.DB
	Diary       *memory.DiaryWriter
	Strategy    *memory.StrategyManager
	Chat        ChatResponder
	BridgeProbe BridgeProbeFunc
	Diag        RuntimeDiagFunc
	LLMCurrent  func() string
	LLMSwitch   func(target string) (string, error)
	LLMStatus   func() map[string]string
	LLMSticky   bool
}

// ChatResponder handles free-form chat messages when chat mode is enabled.
type ChatResponder interface {
	HandleChat(ctx context.Context, userText string) (string, error)
}

// BridgeProbeResult summarizes bridge + EA connectivity diagnostics.
type BridgeProbeResult struct {
	Service       string
	Version       string
	Contract      string
	EAConnected   bool
	EATimestamp   string
	OpenPositions int
}

// BridgeProbeFunc probes runtime bridge/EA connectivity.
type BridgeProbeFunc func(ctx context.Context) (BridgeProbeResult, error)

// RuntimeDiagFunc reports extended runtime diagnostics.
type RuntimeDiagFunc func(ctx context.Context) (string, error)

// Bot wraps the Telegram bot and handles command dispatch.
type Bot struct {
	mu       sync.RWMutex
	b        *bot.Bot
	chatID   int64
	deps     Dependencies
	chatMode bool
}

func logInbound(command string, update *models.Update) {
	if update == nil || update.Message == nil {
		log.Printf("telegram: inbound %s (no message payload)", command)
		return
	}
	log.Printf("telegram: inbound %s chat_id=%d user_id=%d text=%q", command, update.Message.Chat.ID, update.Message.From.ID, update.Message.Text)
}

// New creates a Telegram bot listener.
func New(token string, chatID int64, deps Dependencies) (*Bot, error) {
	if token == "" {
		return nil, fmt.Errorf("telegram: token is required")
	}

	tb := &Bot{chatID: chatID, deps: deps}

	opts := []bot.Option{
		bot.WithDefaultHandler(tb.handleUnknown),
	}

	b, err := bot.New(token, opts...)
	if err != nil {
		return nil, fmt.Errorf("telegram: init error: %w", err)
	}

	b.RegisterHandler(bot.HandlerTypeMessageText, "/status", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleStatus))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleStart))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/halt", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleHalt))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/mode", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleMode))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/auto", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleAuto))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/observe", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleObserve))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/suggest", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleSuggest))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/report", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleReport))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/pairs", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handlePairs))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/pending", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handlePending))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/confidence", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleConfidence))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/provider", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleProvider))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/config", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleConfig))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/rollback", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleRollback))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/chat", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleChatMode))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/handshake", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleHandshake))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/diag", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleDiag))
	b.RegisterHandler(bot.HandlerTypeMessageText, "/help", bot.MatchTypePrefix, tb.wrapAuthorized(tb.handleHelp))

	tb.b = b
	return tb, nil
}

// Start begins polling for Telegram updates (blocking).
func (tb *Bot) Start(ctx context.Context) {
	log.Printf("telegram: bot started, chat_id=%d", tb.chatID)
	tb.b.Start(ctx)
}

// Send sends a message to the configured chat.
func (tb *Bot) Send(ctx context.Context, text string) {
	if tb.chatID == 0 {
		return
	}
	tb.sendReply(ctx, tb.b, tb.chatID, text, true)
}

// sendReply sends a Telegram message and logs failures.
// Markdown mode uses escaped MarkdownV2 to avoid parser errors.
func (tb *Bot) sendReply(ctx context.Context, b *bot.Bot, chatID int64, text string, markdown bool) {
	base := &bot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}

	// Strategy 1: escaped MarkdownV2 for parser compatibility.
	if markdown {
		params := *base
		params.ParseMode = "MarkdownV2"
		params.Text = escapeMarkdownV2(text)
		if _, err := b.SendMessage(ctx, &params); err == nil {
			return
		} else {
			log.Printf("telegram: send markdownv2 failed chat_id=%d err=%v", chatID, err)
		}
	}

	// Strategy 2: plain text fallback.
	params := *base
	params.ParseMode = ""
	params.Text = markdownToPlain(text)
	if _, err := b.SendMessage(ctx, &params); err != nil {
		log.Printf("telegram: send plain fallback failed chat_id=%d err=%v", chatID, err)
	}
}

// markdownToPlain removes lightweight markdown markers so fallback text looks clean.
func markdownToPlain(s string) string {
	r := strings.NewReplacer(
		"`", "",
		"*", "",
		"_", "",
		"[", "",
		"]", "",
		"(", "",
		")", "",
	)
	return r.Replace(s)
}

func escapeMarkdownV2(s string) string {
	// Telegram MarkdownV2 reserved characters.
	re := regexp.MustCompile(`([_*\[\]()~` + "`" + `>#+\-=|{}.!\\])`)
	return re.ReplaceAllString(s, `\$1`)
}

func (tb *Bot) setChatMode(enabled bool) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.chatMode = enabled
}

func (tb *Bot) isChatModeEnabled() bool {
	tb.mu.RLock()
	defer tb.mu.RUnlock()
	return tb.chatMode
}

func (tb *Bot) wrapAuthorized(next func(context.Context, *bot.Bot, *models.Update)) func(context.Context, *bot.Bot, *models.Update) {
	return func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if !tb.isAuthorized(update) {
			tb.logUnauthorized(update)
			return
		}
		next(ctx, b, update)
	}
}

func (tb *Bot) isAuthorized(update *models.Update) bool {
	if update == nil || update.Message == nil {
		return false
	}
	if tb.chatID == 0 {
		return true
	}
	return update.Message.Chat.ID == tb.chatID
}

func (tb *Bot) logUnauthorized(update *models.Update) {
	if update == nil || update.Message == nil {
		log.Printf("telegram: inbound unauthorized (no message payload)")
		return
	}
	userID := int64(0)
	if update.Message.From != nil {
		userID = update.Message.From.ID
	}
	log.Printf(
		"telegram: inbound unauthorized chat_id=%d expected_chat_id=%d user_id=%d text=%q",
		update.Message.Chat.ID,
		tb.chatID,
		userID,
		update.Message.Text,
	)
}

// --- Command Handlers ---

func (tb *Bot) handleStatus(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/status", update)
	stats := tb.deps.Risk.Stats()
	session := tb.deps.Scheduler.CurrentSession()

	msg := fmt.Sprintf("🤖 *PhantomClaw Status*\n\n"+
		"Mode: %s\n"+
		"Session: %s\n"+
		"Open Positions: %d/%d\n"+
		"Daily Loss: $%.2f\n"+
		"Ramp-up: %d/%d profitable trades\n"+
		"Weekend: %v",
		tb.deps.Safety.StatusText(),
		session,
		stats.OpenPositions, stats.MaxPositions,
		stats.DailyLoss,
		stats.ProfitableTrades, stats.RampUpTarget,
		tb.deps.Scheduler.IsWeekend(),
	)

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handleStart(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/start", update)
	msg := "✅ *PhantomClaw connected*\n\nSend `/status` to check runtime state or `/help` for all commands."
	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handleHalt(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/halt", update)
	tb.deps.Safety.SetMode(safety.ModeHalt)
	tb.deps.Risk.SetHalted(true)

	tb.sendReply(ctx, b, update.Message.Chat.ID, "🛑 *HALT ACTIVATED*\nAll trading frozen. Pending orders will be cancelled.\nUse `/mode auto` to resume.", true)
}

func (tb *Bot) handleMode(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/mode", update)
	parts := strings.Fields(update.Message.Text)
	if len(parts) < 2 {
		tb.sendReply(ctx, b, update.Message.Chat.ID, "Usage: `/mode observe|suggest|auto|halt`", true)
		return
	}

	mode, err := safety.ParseMode(parts[1])
	if err != nil {
		tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("❌ %s", err), false)
		return
	}

	tb.applyMode(mode)
	tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("✅ Mode switched to: %s", tb.deps.Safety.StatusText()), false)
}

func (tb *Bot) handleAuto(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/auto", update)
	tb.applyMode(safety.ModeAuto)
	tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("✅ Mode switched to: %s", tb.deps.Safety.StatusText()), false)
}

func (tb *Bot) handleObserve(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/observe", update)
	tb.applyMode(safety.ModeObserve)
	tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("✅ Mode switched to: %s", tb.deps.Safety.StatusText()), false)
}

func (tb *Bot) handleSuggest(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/suggest", update)
	tb.applyMode(safety.ModeSuggest)
	tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("✅ Mode switched to: %s", tb.deps.Safety.StatusText()), false)
}

func (tb *Bot) applyMode(mode safety.Mode) {
	tb.deps.Safety.SetMode(mode)
	if mode == safety.ModeHalt {
		tb.deps.Risk.SetHalted(true)
		return
	}
	tb.deps.Risk.SetHalted(false)
}

func (tb *Bot) handleReport(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/report", update)
	stats := tb.deps.Risk.Stats()

	var diaryText string
	if tb.deps.Diary != nil {
		entries, err := tb.deps.Diary.GetToday()
		if err == nil && len(entries) > 0 {
			var sb strings.Builder
			for _, e := range entries {
				sb.WriteString(fmt.Sprintf("• [%s] %s\n", e.EntryType, e.Content))
			}
			diaryText = sb.String()
		}
	}
	if diaryText == "" {
		diaryText = "_No diary entries today_"
	}

	msg := fmt.Sprintf("📊 *Daily Report*\n\n"+
		"Daily P&L: -$%.2f\n"+
		"Open Positions: %d\n"+
		"Profitable Trades: %d\n"+
		"Session: %s\n\n"+
		"*Today's Diary:*\n%s",
		stats.DailyLoss,
		stats.OpenPositions,
		stats.ProfitableTrades,
		tb.deps.Scheduler.CurrentSession(),
		diaryText,
	)

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handlePairs(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/pairs", update)
	var msg strings.Builder
	msg.WriteString("📈 *Active Pairs*\n\n")

	if tb.deps.Memory != nil {
		rows, err := tb.deps.Memory.QueryRows(
			"SELECT symbol, bias, win_rate_7d, avg_pnl_7d FROM pair_state ORDER BY updated_at DESC LIMIT 10",
		)
		if err == nil {
			defer rows.Close()
			found := false
			for rows.Next() {
				var symbol, bias string
				var winRate, avgPnl float64
				rows.Scan(&symbol, &bias, &winRate, &avgPnl)
				msg.WriteString(fmt.Sprintf("• %s: %s (WR: %.0f%%, PnL: $%.2f)\n",
					symbol, bias, winRate*100, avgPnl))
				found = true
			}
			if !found {
				msg.WriteString("_No pair state data yet — trades needed_\n")
			}
		}
	} else {
		msg.WriteString("XAUUSD, EURUSD, USDJPY, GBPUSD\n")
	}

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg.String(), true)
}

func (tb *Bot) handlePending(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/pending", update)
	tb.sendReply(ctx, b, update.Message.Chat.ID, "📋 *Pending Orders*\n\n_Pending orders are managed by the EA. Check MT5 terminal for live status._", true)
}

func (tb *Bot) handleConfidence(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/confidence", update)
	conf := skills.ScoreConfidence(skills.ConfidenceInput{
		Session: string(tb.deps.Scheduler.CurrentSession()),
	})

	msg := fmt.Sprintf("🎯 *Confidence Score*\n\n"+
		"Score: %d/100\n"+
		"Action: %s\n"+
		"Lot Factor: %.1fx\n\n"+
		"_Score based on current session quality. Full 7-factor scoring requires live market data._",
		conf.Score, conf.Action, conf.LotFactor,
	)

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handleConfig(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/config", update)
	stats := tb.deps.Risk.Stats()
	msg := fmt.Sprintf("⚙️ *Risk Config*\n\n"+
		"Max Open Positions: %d\n"+
		"Ramp-up Target: %d trades\n"+
		"Halted: %v",
		stats.MaxPositions,
		stats.RampUpTarget,
		stats.Halted,
	)

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, true)
}

func (tb *Bot) handleProvider(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/provider", update)
	if tb.deps.LLMCurrent == nil {
		tb.sendReply(ctx, b, update.Message.Chat.ID, "❌ Provider control unavailable: LLM router not configured.", false)
		return
	}

	parts := strings.Fields(update.Message.Text)
	if len(parts) < 2 || strings.EqualFold(parts[1], "status") {
		current := tb.deps.LLMCurrent()
		if strings.TrimSpace(current) == "" {
			current = "unknown"
		}

		var sb strings.Builder
		sb.WriteString("LLM Provider\n\n")
		sb.WriteString(fmt.Sprintf("Current: %s\n", current))
		sb.WriteString(fmt.Sprintf("Sticky primary: %t\n", tb.deps.LLMSticky))

		if tb.deps.LLMStatus != nil {
			status := tb.deps.LLMStatus()
			if len(status) > 0 {
				keys := make([]string, 0, len(status))
				for k := range status {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				sb.WriteString("\nProviders:\n")
				for _, k := range keys {
					sb.WriteString(fmt.Sprintf("- %s: %s\n", k, status[k]))
				}
			}
		}
		sb.WriteString("\nUsage: /provider <name_or_alias>")
		tb.sendReply(ctx, b, update.Message.Chat.ID, sb.String(), false)
		return
	}

	if tb.deps.LLMSwitch == nil {
		tb.sendReply(ctx, b, update.Message.Chat.ID, "❌ Provider switching unavailable in current runtime.", false)
		return
	}

	active, err := tb.deps.LLMSwitch(parts[1])
	if err != nil {
		tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("❌ Provider switch failed: %v", err), false)
		return
	}

	tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("✅ LLM provider switched. Active provider: %s", active), false)
}

func (tb *Bot) handleRollback(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/rollback", update)
	if tb.deps.Strategy == nil {
		tb.sendReply(ctx, b, update.Message.Chat.ID, "❌ Strategy manager not configured", false)
		return
	}

	versions, err := tb.deps.Strategy.ListVersions(5)
	if err != nil || len(versions) == 0 {
		tb.sendReply(ctx, b, update.Message.Chat.ID, "📜 No strategy versions available yet.", false)
		return
	}

	var sb strings.Builder
	sb.WriteString("📜 *Strategy Versions*\n\n")
	for _, v := range versions {
		sb.WriteString(fmt.Sprintf("v%d — %s (%s)\n", v.Version, v.PatchType, v.Reason))
	}
	sb.WriteString("\n_Use `/rollback N` to revert to version N_")

	// Check if a version number was provided
	parts := strings.Fields(update.Message.Text)
	if len(parts) >= 2 {
		var targetVersion int
		fmt.Sscanf(parts[1], "%d", &targetVersion)
		if targetVersion > 0 {
			if err := tb.deps.Strategy.Rollback(targetVersion); err != nil {
				tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("❌ Rollback failed: %v", err), false)
				return
			}
			tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("✅ Rolled back to strategy v%d", targetVersion), true)
			return
		}
	}

	tb.sendReply(ctx, b, update.Message.Chat.ID, sb.String(), true)
}

func (tb *Bot) handleHelp(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/help", update)
	msg := "🤖 *PhantomClaw Commands*\n\n" +
		"`/status` — Mode, positions, PnL, session\n" +
		"`/mode observe|suggest|auto|halt` — Switch mode\n" +
		"`/auto` `/observe` `/suggest` — Quick mode aliases\n" +
		"`/halt` — Emergency stop\n" +
		"`/provider [status|name]` — Show or switch LLM provider\n" +
		"`/report` — Daily summary + diary\n" +
		"`/pairs` — Active pairs + bias\n" +
		"`/pending` — Pending orders (check MT5)\n" +
		"`/confidence` — Current confidence score\n" +
		"`/rollback [N]` — Strategy versions / rollback\n" +
		"`/chat on|off|status` — Toggle intelligent chat replies\n" +
		"`/handshake` — Verify Telegram, bridge, and EA connectivity\n" +
		"`/diag` — Extended runtime diagnostics\n" +
		"`/config` — Risk config\n" +
		"`/help` — This message"

	tb.sendReply(ctx, b, update.Message.Chat.ID, msg, false)
}

func (tb *Bot) handleHandshake(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/handshake", update)

	var sb strings.Builder
	sb.WriteString("Handshake Check\n\n")
	sb.WriteString("Telegram: connected (command received)\n")

	if tb.deps.BridgeProbe == nil {
		sb.WriteString("Server: probe unavailable\n")
		sb.WriteString("EA -> Server: unknown\n")
		tb.sendReply(ctx, b, update.Message.Chat.ID, sb.String(), false)
		return
	}

	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	result, err := tb.deps.BridgeProbe(probeCtx)
	if err != nil {
		sb.WriteString(fmt.Sprintf("Server: probe failed (%v)\n", err))
		sb.WriteString("EA -> Server: unknown\n")
		tb.sendReply(ctx, b, update.Message.Chat.ID, sb.String(), false)
		return
	}

	sb.WriteString(fmt.Sprintf("Server: connected (%s v%s, contract=%s)\n", result.Service, result.Version, result.Contract))
	if result.EAConnected {
		sb.WriteString(fmt.Sprintf("EA -> Server: connected (last snapshot %s, open_positions=%d)\n", result.EATimestamp, result.OpenPositions))
	} else {
		sb.WriteString("EA -> Server: waiting for first snapshot\n")
	}

	tb.sendReply(ctx, b, update.Message.Chat.ID, sb.String(), false)
}

func (tb *Bot) handleDiag(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/diag", update)

	stats := tb.deps.Risk.Stats()
	var sb strings.Builder
	sb.WriteString("Diagnostics\n\n")
	sb.WriteString(fmt.Sprintf("Mode: %s\n", tb.deps.Safety.CurrentMode()))
	sb.WriteString(fmt.Sprintf("Session: %s\n", tb.deps.Scheduler.CurrentSession()))
	sb.WriteString(fmt.Sprintf("Open positions: %d/%d\n", stats.OpenPositions, stats.MaxPositions))
	sb.WriteString(fmt.Sprintf("Daily loss: %.2f\n", stats.DailyLoss))
	sb.WriteString(fmt.Sprintf("Chat mode: %t\n", tb.isChatModeEnabled()))

	if tb.deps.Memory != nil {
		if entries, err := tb.deps.Memory.ListActivePendingDecisions(20); err == nil {
			sb.WriteString(fmt.Sprintf("Pending decision queue: %d\n", len(entries)))
		}
	}

	if tb.deps.Diag != nil {
		diagCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		extra, err := tb.deps.Diag(diagCtx)
		if err != nil {
			sb.WriteString(fmt.Sprintf("Diag source: error (%v)\n", err))
		} else if strings.TrimSpace(extra) != "" {
			sb.WriteString("\n")
			sb.WriteString(extra)
			if !strings.HasSuffix(extra, "\n") {
				sb.WriteString("\n")
			}
		}
	}

	tb.sendReply(ctx, b, update.Message.Chat.ID, sb.String(), false)
}

func (tb *Bot) handleChatMode(ctx context.Context, b *bot.Bot, update *models.Update) {
	logInbound("/chat", update)
	parts := strings.Fields(update.Message.Text)
	if len(parts) < 2 {
		status := "off"
		if tb.isChatModeEnabled() {
			status = "on"
		}
		tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("💬 Chat mode is *%s*.\nUsage: `/chat on|off|status`", status), true)
		return
	}

	switch strings.ToLower(strings.TrimSpace(parts[1])) {
	case "on":
		if tb.deps.Chat == nil {
			tb.sendReply(ctx, b, update.Message.Chat.ID, "❌ Chat mode unavailable: agent brain is not configured.", false)
			return
		}
		tb.setChatMode(true)
		tb.sendReply(ctx, b, update.Message.Chat.ID, "✅ Chat mode enabled. Non-command messages will be answered by PhantomClaw brain.", false)
	case "off":
		tb.setChatMode(false)
		tb.sendReply(ctx, b, update.Message.Chat.ID, "✅ Chat mode disabled.", false)
	case "status":
		status := "off"
		if tb.isChatModeEnabled() {
			status = "on"
		}
		tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("💬 Chat mode is %s.", status), false)
	default:
		tb.sendReply(ctx, b, update.Message.Chat.ID, "Usage: `/chat on|off|status`", true)
	}
}

func (tb *Bot) handleUnknown(ctx context.Context, b *bot.Bot, update *models.Update) {
	if !tb.isAuthorized(update) {
		tb.logUnauthorized(update)
		return
	}
	logInbound("unknown", update)
	if update.Message == nil {
		return
	}

	if tb.isChatModeEnabled() && tb.deps.Chat != nil && !strings.HasPrefix(strings.TrimSpace(update.Message.Text), "/") {
		chatCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		reply, err := tb.deps.Chat.HandleChat(chatCtx, update.Message.Text)
		if err != nil {
			tb.sendReply(ctx, b, update.Message.Chat.ID, fmt.Sprintf("❌ Chat error: %v", err), false)
			return
		}
		if strings.TrimSpace(reply) == "" {
			tb.sendReply(ctx, b, update.Message.Chat.ID, "I don't have a useful response right now.", false)
			return
		}
		tb.sendReply(ctx, b, update.Message.Chat.ID, reply, false)
		return
	}

	tb.sendReply(ctx, b, update.Message.Chat.ID, "🤖 Bot is online. Use /help to see available commands.", false)
}
