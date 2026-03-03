package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/agent"
	"github.com/nzkbuild/PhantomClaw/internal/alerts"
	"github.com/nzkbuild/PhantomClaw/internal/bridge"
	"github.com/nzkbuild/PhantomClaw/internal/config"
	"github.com/nzkbuild/PhantomClaw/internal/health"
	"github.com/nzkbuild/PhantomClaw/internal/llm"
	"github.com/nzkbuild/PhantomClaw/internal/logging"
	"github.com/nzkbuild/PhantomClaw/internal/market"
	"github.com/nzkbuild/PhantomClaw/internal/memory"
	"github.com/nzkbuild/PhantomClaw/internal/risk"
	"github.com/nzkbuild/PhantomClaw/internal/safety"
	"github.com/nzkbuild/PhantomClaw/internal/scheduler"
	"github.com/nzkbuild/PhantomClaw/internal/skills"
	"github.com/nzkbuild/PhantomClaw/internal/telegram"

	"go.uber.org/zap"
)

var version = "3.0.0"

type alertSender interface {
	Send(ctx context.Context, text string)
}

func makeSessionAlertSender(sender alertSender, logger *zap.SugaredLogger) func(context.Context, string) {
	return func(ctx context.Context, text string) {
		logger.Infow("alert: sending", "text_len", len(text))
		if sender != nil {
			sender.Send(ctx, text)
		}
	}
}

func makeBridgeProbe(baseURL, authToken string) telegram.BridgeProbeFunc {
	return func(ctx context.Context) (telegram.BridgeProbeResult, error) {
		client := &http.Client{Timeout: 1500 * time.Millisecond}
		out := telegram.BridgeProbeResult{}

		healthReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
		if err != nil {
			return out, err
		}
		healthResp, err := client.Do(healthReq)
		if err != nil {
			return out, err
		}
		defer healthResp.Body.Close()
		if healthResp.StatusCode != http.StatusOK {
			return out, fmt.Errorf("health status %d", healthResp.StatusCode)
		}

		var healthPayload struct {
			Status   string `json:"status"`
			Service  string `json:"service"`
			Version  string `json:"version"`
			Contract string `json:"contract"`
		}
		if err := json.NewDecoder(healthResp.Body).Decode(&healthPayload); err != nil {
			return out, err
		}
		if strings.ToLower(strings.TrimSpace(healthPayload.Status)) != "ok" {
			return out, fmt.Errorf("health status=%q", healthPayload.Status)
		}
		out.Service = healthPayload.Service
		out.Version = healthPayload.Version
		out.Contract = healthPayload.Contract

		accountReq, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/account", nil)
		if err != nil {
			return out, err
		}
		if strings.TrimSpace(authToken) != "" {
			accountReq.Header.Set("X-Phantom-Bridge-Token", authToken)
		}
		if strings.TrimSpace(out.Contract) != "" {
			accountReq.Header.Set("X-Phantom-Bridge-Contract", out.Contract)
		}

		accountResp, err := client.Do(accountReq)
		if err != nil {
			return out, err
		}
		defer accountResp.Body.Close()
		if accountResp.StatusCode == http.StatusNotFound {
			return out, nil
		}
		if accountResp.StatusCode == http.StatusUnauthorized {
			return out, errors.New("account unauthorized (bridge token mismatch)")
		}
		if accountResp.StatusCode != http.StatusOK {
			return out, fmt.Errorf("account status %d", accountResp.StatusCode)
		}

		var accountPayload struct {
			OpenPositions int    `json:"open_positions"`
			Timestamp     string `json:"timestamp"`
		}
		if err := json.NewDecoder(accountResp.Body).Decode(&accountPayload); err != nil {
			return out, err
		}
		out.EAConnected = true
		out.EATimestamp = accountPayload.Timestamp
		out.OpenPositions = accountPayload.OpenPositions
		return out, nil
	}
}

func makeRuntimeDiag(baseURL, authToken string, providerCount int, db *memory.DB) telegram.RuntimeDiagFunc {
	probe := makeBridgeProbe(baseURL, authToken)
	return func(ctx context.Context) (string, error) {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("LLM providers configured: %d\n", providerCount))
		sb.WriteString(fmt.Sprintf("Bridge auth enabled: %t\n", strings.TrimSpace(authToken) != ""))

		if db != nil {
			if jobs, err := db.ListPendingCronJobs(); err == nil {
				sb.WriteString(fmt.Sprintf("Pending cron jobs: %d\n", len(jobs)))
			}
		}

		result, err := probe(ctx)
		if err != nil {
			sb.WriteString(fmt.Sprintf("Bridge probe: error (%v)\n", err))
			return sb.String(), nil
		}
		sb.WriteString(fmt.Sprintf("Bridge service: %s v%s (contract=%s)\n", result.Service, result.Version, result.Contract))
		if result.EAConnected {
			sb.WriteString(fmt.Sprintf("EA snapshot: connected (%s, open_positions=%d)\n", result.EATimestamp, result.OpenPositions))
		} else {
			sb.WriteString("EA snapshot: waiting for first snapshot\n")
		}
		return sb.String(), nil
	}
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	fmt.Printf("🐾 PhantomClaw v%s starting...\n", version)

	// --- Load config ---
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// --- Structured logging (zap) ---
	zapLogger, err := logging.New(cfg.Memory.LogDir, cfg.Bot.LogLevel)
	if err != nil {
		log.Fatalf("logging: %v", err)
	}
	defer zapLogger.Sync()
	zap.ReplaceGlobals(zapLogger)
	restoreStdLog := zap.RedirectStdLog(zapLogger)
	defer restoreStdLog()
	logger := zapLogger.Sugar()
	logger.Infow("config loaded", "mode", cfg.Bot.Mode, "tz", cfg.Bot.Timezone, "pairs", cfg.Pairs)

	// --- Single-instance guard ---
	lockPath := filepath.Join(filepath.Dir(cfg.Memory.DBPath), "phantomclaw.lock")
	lock, err := acquireSingleInstanceLock(lockPath)
	if err != nil {
		logger.Fatalf("startup guard: %v", err)
	}
	defer releaseSingleInstanceLock(lock, lockPath)
	logger.Infow("startup guard: lock acquired", "path", lockPath)

	// --- Initialize SQLite ---
	db, err := memory.NewDB(cfg.Memory.DBPath)
	if err != nil {
		logger.Fatalf("memory: %v", err)
	}
	defer db.Close()
	logger.Info("memory: SQLite initialized")

	// --- Risk engine ---
	riskEngine := risk.NewEngine(cfg.Risk)
	logger.Info("risk: engine initialized")

	// --- Safety mode manager ---
	safetyMgr, err := safety.NewManager(
		cfg.Bot.Mode,
		safety.SessionWindow{
			LearningStart: cfg.Sessions.LearningStart,
			LearningEnd:   cfg.Sessions.LearningEnd,
		},
		cfg.Bot.Timezone,
		func() {
			logger.Warn("safety: HALT triggered — closing all positions")
			riskEngine.SetHalted(true)
		},
	)
	if err != nil {
		logger.Fatalf("safety: %v", err)
	}
	logger.Infow("safety: mode set", "mode", safetyMgr.CurrentMode())

	// --- Error recovery ---
	recovery := health.NewRecovery(func(component, action string) {
		logger.Warnw("recovery: action triggered", "component", component, "action", action)
		if action == "switch_provider" && cfg.LLM.StickyPrimary {
			logger.Infow("recovery: switch_provider ignored (sticky primary enabled)", "component", component)
			return
		}
		if action == "halt" {
			safetyMgr.SetMode(safety.ModeHalt)
		}
	})

	// --- Rate limiter ---
	rateLimiter := health.NewRateLimiter()

	// --- Skills registry ---
	bridgeURL := fmt.Sprintf("http://%s:%d", cfg.Bridge.Host, cfg.Bridge.Port)
	skillReg := skills.NewRegistry()
	skills.RegisterMVPSkills(skillReg, bridgeURL, nil) // cron_add registered after scheduler init
	logger.Infow("skills: registered", "count", len(skillReg.Names()), "names", skillReg.Names())

	// --- LLM providers + router (config-driven) ---
	var providers []llm.Provider
	providerByName := map[string]llm.Provider{}

	for _, entry := range cfg.LLM.Providers {
		if shouldSkipProvider(entry, logger) {
			continue
		}

		var p llm.Provider
		var err error

		switch entry.Type {
		case "claude":
			p, err = llm.NewClaude(llm.ProviderConfig{APIKey: entry.APIKey, Model: entry.Model})
		case "openai":
			p, err = llm.NewOpenAI(llm.ProviderConfig{APIKey: entry.APIKey, Model: entry.Model})
		case "openai_compat":
			if entry.BaseURL == "" {
				logger.Warnw("llm: skipping provider (no base_url)", "name", entry.Name)
				continue
			}
			p, err = llm.NewGeneric(llm.GenericConfig{
				Name:    entry.Name,
				BaseURL: entry.BaseURL,
				APIKey:  entry.APIKey,
				Model:   entry.Model,
			})
		default:
			logger.Warnw("llm: unknown provider type, skipping", "name", entry.Name, "type", entry.Type)
			continue
		}

		if err != nil {
			logger.Warnw("llm: failed to init provider", "name", entry.Name, "error", err)
			continue
		}
		providers = append(providers, p)
		providerByName[p.Name()] = p
		providerByName[strings.ToLower(p.Name())] = p
		logger.Infow("llm: provider ready", "name", entry.Name, "type", entry.Type, "model", entry.Model)
	}

	// Build smart router with error-aware fallback + aliases
	var llmProvider llm.Provider
	var llmRouter *llm.Router
	if len(providers) > 0 {
		// Reorder: put primary first
		primaryName := cfg.LLM.Primary
		// Resolve alias if needed
		if resolved, ok := cfg.LLM.Aliases[primaryName]; ok {
			primaryName = resolved
		}
		ordered := reorderProviders(providers, primaryName)

		llmRouter = llm.NewRouter(llm.RouterConfig{
			Providers:     ordered,
			Aliases:       cfg.LLM.Aliases,
			StickyPrimary: cfg.LLM.StickyPrimary,
		})
		llmProvider = llmRouter
		logger.Infow("llm: router initialized",
			"primary", ordered[0].Name(),
			"total", len(ordered),
			"aliases", cfg.LLM.Aliases,
			"sticky_primary", cfg.LLM.StickyPrimary,
		)
	} else {
		logger.Warn("llm: NO providers configured — agent brain disabled")
	}

	// --- Data connectors ---
	newsFetcher := market.NewNewsFetcher(db, cfg.Market.FailPolicy, rateLimiter, recovery)
	sentimentFetcher := market.NewSentimentFetcher(db, rateLimiter, recovery)
	cotFetcher := market.NewCOTFetcher(db, rateLimiter, recovery)
	logger.Info("market: data connectors initialized (news, sentiment, COT)")

	// --- Memory helpers ---
	echoRecall := memory.NewEchoRecall(db)
	diaryWriter := memory.NewDiaryWriter(db)
	var strategyMgr *memory.StrategyManager
	if llmProvider != nil {
		strategyMgr = memory.NewStrategyManager(db, llmProvider)
	}
	logger.Info("memory: echo recall, diary, strategy manager initialized")

	// --- Guards ---
	corrGuard := skills.NewCorrelationGuard(0.7)
	spreadFilter := skills.NewSpreadFilter(50, 2.0)
	logger.Info("guards: correlation + spread filter initialized")

	// --- Session scheduler ---
	sched, err := scheduler.New(cfg.Sessions, cfg.Bot.Timezone, scheduler.Callbacks{
		OnTokyoOpen: func() {
			logger.Info("session: Tokyo open — RESEARCH mode")
		},
		OnPreLondon: func() {
			logger.Info("session: Pre-London alert — 15 min to trading")
		},
		OnLondonOpen: func() {
			logger.Info("session: London open — TRADING mode")
		},
		OnHardStop: func() {
			logger.Info("session: Hard stop — resetting daily counters")
			riskEngine.ResetDaily()
			db.ClearSessionRAM()
		},
	})
	if err != nil {
		logger.Fatalf("scheduler: %v", err)
	}
	sched.Start()
	defer sched.Stop()

	// Register cron_add tool now that scheduler exists
	cronDeps := skills.CronDeps{
		Scheduler: sched,
		DB:        db,
		OnWake: func(pair, reason string) {
			logger.Infow("cron_add: agent recheck triggered", "pair", pair, "reason", reason)
		},
	}
	if err := skills.ReplayPendingCronJobs(cronDeps); err != nil {
		logger.Warnw("cron_add: failed to replay pending jobs", "error", err)
	} else {
		logger.Info("cron_add: replayed pending durable jobs")
	}
	skillReg.Register(skills.CronAddSkill(cronDeps))

	// --- Session store (conversation history) ---
	sessionsDir := cfg.Memory.SessionsDir
	sessionStore, err := memory.NewSessionStore(sessionsDir, cfg.Bot.Timezone)
	if err != nil {
		logger.Warnw("sessions: failed to create store", "error", err)
	} else {
		logger.Infow("sessions: store ready", "dir", sessionsDir)
	}

	// --- Heartbeat ---
	var heartbeat *scheduler.Heartbeat
	if cfg.Heartbeat.Enabled {
		heartbeat = scheduler.NewHeartbeat(scheduler.HeartbeatConfig{
			IntervalMin: cfg.Heartbeat.IntervalMin,
			HealthCheck: func() error {
				if db == nil {
					return fmt.Errorf("database not initialized")
				}
				return nil
			},
			Alerter: func(msg string) {
				logger.Warnw("heartbeat alert", "message", msg)
			},
		})
		heartbeat.Start()
		defer heartbeat.Stop()
	}

	// --- Agent brain ---
	var brain *agent.Agent
	if llmProvider != nil {
		brain = agent.New(agent.Deps{
			LLM:         llmProvider,
			Skills:      skillReg,
			Memory:      db,
			Risk:        riskEngine,
			Safety:      safetyMgr,
			Scheduler:   sched,
			Pairs:       cfg.Pairs,
			Sessions:    sessionStore,
			Correlation: corrGuard,
			Spread:      spreadFilter,
			News:        newsFetcher,
			Sentiment:   sentimentFetcher,
			COT:         cotFetcher,
			Strategy:    strategyMgr,
			Echo:        echoRecall,
			Diary:       diaryWriter,
		})
		logger.Info("agent: brain initialized with full integrations + conversation memory")
	}

	// --- MT5 REST bridge ---
	bridge.SetVersion(version)
	bridgeAuthToken := strings.TrimSpace(cfg.Bridge.AuthToken)
	if bridgeAuthToken == "" {
		logger.Warn("bridge: auth token not set; bridge endpoints are open to local callers")
	} else {
		logger.Info("bridge: auth token enabled for bridge endpoints")
	}

	bridgeServer := bridge.NewServer(
		cfg.Bridge.Host,
		cfg.Bridge.Port,
		func(ctx context.Context, req *bridge.SignalRequest) *bridge.SignalResponse {
			recovery.RecordSuccess("bridge")

			if !rateLimiter.Allow("llm") {
				wait := rateLimiter.WaitTime("llm")
				err := fmt.Errorf("llm rate limited, retry in %s", wait)
				recovery.RecordError("llm", err)
				return &bridge.SignalResponse{Action: "HOLD", Reason: err.Error()}
			}

			// Reconcile risk engine snapshot from MT5 before evaluating the new signal.
			riskEngine.SyncAccountSnapshot(req.Equity, req.OpenPos)
			if brain == nil {
				recovery.RecordError("llm", fmt.Errorf("agent brain not configured"))
				return &bridge.SignalResponse{Action: "HOLD", Reason: "agent brain not configured (no LLM API key)"}
			}
			resp := brain.HandleSignal(ctx, req)
			if resp != nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(resp.Reason)), "llm error") {
				recovery.RecordError("llm", fmt.Errorf("%s", resp.Reason))
			} else {
				recovery.RecordSuccess("llm")
			}
			return resp
		},
		func(req *bridge.TradeResultRequest) {
			recovery.RecordSuccess("bridge")
			logger.Infow("trade-result", "symbol", req.Symbol, "direction", req.Direction, "pnl", req.PnL)
			if brain != nil {
				brain.HandleTradeResult(context.Background(), req)
			} else {
				recovery.RecordError("llm", fmt.Errorf("trade-result handled without brain"))
				riskEngine.RecordTradeClose(req.PnL)
			}
		},
		db,
		bridgeAuthToken,
	)
	bridgeServer.SetSignalTimeout(cfg.Bridge.SignalTimeoutDuration())
	logger.Infow("bridge: signal timeout configured", "timeout_ms", cfg.Bridge.SignalTimeoutDuration().Milliseconds())

	// --- Telegram bot ---
	var tgBot *telegram.Bot
	if cfg.Telegram.Token != "" {
		llmCurrent := func() string { return "" }
		llmSwitch := func(string) (string, error) {
			return "", fmt.Errorf("llm router not configured")
		}
		llmStatus := func() map[string]string { return nil }
		if llmRouter != nil {
			llmCurrent = func() string {
				return strings.TrimPrefix(llmRouter.Name(), "router:")
			}
			llmStatus = llmRouter.ProviderStatus
			llmSwitch = func(target string) (string, error) {
				requested := strings.TrimSpace(target)
				if requested == "" {
					return "", fmt.Errorf("provider name or alias is required")
				}
				resolved := llmRouter.Resolve(requested)
				provider, ok := providerByName[resolved]
				if !ok {
					provider, ok = providerByName[strings.ToLower(resolved)]
				}
				if !ok {
					known := make([]string, 0, len(providerByName))
					seen := map[string]struct{}{}
					for _, p := range providers {
						if _, exists := seen[p.Name()]; exists {
							continue
						}
						seen[p.Name()] = struct{}{}
						known = append(known, p.Name())
					}
					return "", fmt.Errorf("unknown provider/alias '%s' (resolved: '%s'). available: %s", requested, resolved, strings.Join(known, ", "))
				}
				llmRouter.SetPrimary(provider)
				return strings.TrimPrefix(llmRouter.Name(), "router:"), nil
			}
		}

		tgBot, err = telegram.New(cfg.Telegram.Token, cfg.Telegram.ChatID, telegram.Dependencies{
			Safety:      safetyMgr,
			Risk:        riskEngine,
			Scheduler:   sched,
			Memory:      db,
			Diary:       diaryWriter,
			Strategy:    strategyMgr,
			Chat:        brain,
			BridgeProbe: makeBridgeProbe(bridgeURL, bridgeAuthToken),
			Diag:        makeRuntimeDiag(bridgeURL, bridgeAuthToken, len(providers), db),
			LLMCurrent:  llmCurrent,
			LLMSwitch:   llmSwitch,
			LLMStatus:   llmStatus,
			LLMSticky:   cfg.LLM.StickyPrimary,
		})
		if err != nil {
			logger.Fatalf("telegram: %v", err)
		}
	} else {
		logger.Warn("telegram: skipped (no token configured)")
	}

	// --- Health monitor ---
	healthMonitor := health.NewMonitor(5*time.Minute, func(component string, status health.Status, message string) {
		logger.Warnw("health: status change", "component", component, "status", status, "message", message)
	})
	healthMonitor.Register("memory", func() health.Status {
		if err := db.QueryRow("SELECT 1").Err(); err != nil {
			return health.StatusDown
		}
		return health.StatusOK
	})
	healthMonitor.Register("bridge", func() health.Status {
		healthURL := fmt.Sprintf("http://%s:%d/health", cfg.Bridge.Host, cfg.Bridge.Port)
		req, err := http.NewRequest(http.MethodGet, healthURL, nil)
		if err != nil {
			return health.StatusDown
		}
		client := &http.Client{Timeout: 750 * time.Millisecond}
		resp, err := client.Do(req)
		if err != nil {
			return health.StatusDown
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return health.StatusDown
		}
		var payload struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return health.StatusDegraded
		}
		if strings.ToLower(strings.TrimSpace(payload.Status)) != "ok" {
			return health.StatusDegraded
		}
		return health.StatusOK
	})
	healthMonitor.Start()
	defer healthMonitor.Stop()

	// --- Session alerts ---
	var sessionAlerts *alerts.SessionAlerts
	if tgBot != nil {
		sessionAlerts = alerts.NewSessionAlerts(
			makeSessionAlertSender(tgBot, logger),
			cfg.Bot.Timezone,
		)
		sessionAlerts.Start()
		defer sessionAlerts.Stop()
		logger.Info("alerts: session alerts started")
	}

	// --- Start services ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := bridgeServer.Start(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("bridge: %v", err)
		}
	}()

	if tgBot != nil {
		go tgBot.Start(ctx)
	}

	logger.Infow("🐾 PhantomClaw is running",
		"version", version,
		"mode", cfg.Bot.Mode,
		"pairs", cfg.Pairs,
		"llm_providers", len(providers),
		"bridge", fmt.Sprintf("%s:%d", cfg.Bridge.Host, cfg.Bridge.Port),
	)

	// --- Graceful shutdown ---
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutdown: stopping services...")
	cancel()
	bridgeServer.Stop()
	logger.Info("shutdown: complete")
}

// reorderProviders puts the provider with the given name first, keeping the rest in order.
func reorderProviders(providers []llm.Provider, primaryName string) []llm.Provider {
	var primary llm.Provider
	var rest []llm.Provider
	for _, p := range providers {
		if p.Name() == primaryName && primary == nil {
			primary = p
		} else {
			rest = append(rest, p)
		}
	}
	if primary != nil {
		return append([]llm.Provider{primary}, rest...)
	}
	return providers // primary not found, keep original order
}

func shouldSkipProvider(entry config.LLMProviderEntry, logger *zap.SugaredLogger) bool {
	if strings.TrimSpace(entry.Name) == "" {
		logger.Warnw("llm: skipping provider (missing name)", "type", entry.Type)
		return true
	}

	if strings.TrimSpace(entry.Model) == "" {
		logger.Warnw("llm: skipping provider (missing model)", "name", entry.Name)
		return true
	}

	apiKey := strings.TrimSpace(entry.APIKey)
	switch entry.Type {
	case "openai_compat":
		baseURL := strings.TrimSpace(entry.BaseURL)
		if baseURL == "" {
			logger.Warnw("llm: skipping provider (no base_url)", "name", entry.Name)
			return true
		}
		// Allow keyless local providers (e.g. Ollama), but require key for remote APIs.
		if apiKey == "" && !isLocalProviderEndpoint(baseURL) {
			logger.Warnw("llm: skipping provider (missing api_key for remote endpoint)", "name", entry.Name, "base_url", baseURL)
			return true
		}
		return false
	case "claude", "openai":
		if apiKey == "" {
			logger.Warnw("llm: skipping provider (missing api_key)", "name", entry.Name, "type", entry.Type)
			return true
		}
		return false
	default:
		return false
	}
}

func isLocalProviderEndpoint(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return strings.HasSuffix(host, ".local")
	}
}
