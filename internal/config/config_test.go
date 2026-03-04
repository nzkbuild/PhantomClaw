package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig() *Config {
	return &Config{
		Bot: BotConfig{
			Mode:     "AUTO",
			Timezone: "Asia/Kuala_Lumpur",
			LogLevel: "info",
		},
		LLM: LLMConfig{
			Primary: "ollama",
			Providers: []LLMProviderEntry{
				{
					Name:    "ollama",
					Type:    "openai_compat",
					BaseURL: "http://localhost:11434/v1",
					Model:   "qwen3",
				},
			},
		},
		Bridge: BridgeConfig{
			Port:            8765,
			SignalTimeoutMs: 30000,
		},
		Market: MarketConfig{FailPolicy: "fail_open"},
		Risk: RiskConfig{
			MaxLotSize:        0.1,
			MaxDailyLossUSD:   100,
			MaxOpenPositions:  3,
			MaxDrawdownPct:    10,
			SuggestTimeoutSec: 180,
			MinTradeInterval:  "15m",
			RampUpTrades:      10,
			RampUpLotPct:      0.5,
		},
		Sessions: SessionConfig{
			TokyoOpen:     "08:00",
			PreLondon:     "14:45",
			LondonOpen:    "15:00",
			NYOverlapEnd:  "00:00",
			LearningStart: "00:00",
			LearningEnd:   "08:00",
		},
		Memory: MemoryConfig{
			DBPath:      "data/phantom.db",
			LogDir:      "data/logs",
			SessionsDir: "data/sessions",
		},
		Pairs: []string{"XAUUSD"},
	}
}

func TestValidateRejectsInvalidTimezone(t *testing.T) {
	cfg := validConfig()
	cfg.Bot.Timezone = "Mars/Olympus"

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected invalid timezone validation error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timezone") {
		t.Fatalf("expected timezone error, got: %v", err)
	}
}

func TestValidateAllowsRolloverSessionOrdering(t *testing.T) {
	cfg := validConfig()
	cfg.Sessions = SessionConfig{
		TokyoOpen:     "08:00",
		PreLondon:     "14:45",
		LondonOpen:    "15:00",
		NYOverlapEnd:  "00:00", // next day rollover is valid
		LearningStart: "00:00",
		LearningEnd:   "08:00",
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("expected rollover schedule to be valid, got: %v", err)
	}
}

func TestLoadAppliesSecretsFile(t *testing.T) {
	const envKey = "PHANTOM_TELEGRAM_TOKEN"
	oldValue, had := os.LookupEnv(envKey)
	_ = os.Unsetenv(envKey)
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(envKey, oldValue)
		} else {
			_ = os.Unsetenv(envKey)
		}
	})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`
llm:
  primary: ollama
  providers:
    - name: ollama
      type: openai_compat
      base_url: "http://localhost:11434/v1"
      model: "qwen3"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".secrets"), []byte(`
PHANTOM_TELEGRAM_TOKEN=from_secrets_file
`), 0o644); err != nil {
		t.Fatalf("write secrets: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got := cfg.Telegram.Token; got != "from_secrets_file" {
		t.Fatalf("telegram token=%q, want from_secrets_file", got)
	}
}

func TestValidateOpsAlertsRangeChecks(t *testing.T) {
	cfg := validConfig()
	cfg.OpsAlerts = OpsAlertsConfig{
		Enabled:           true,
		PollIntervalSec:   1,
		ProbeTimeoutMs:    1500,
		DegradeForSec:     20,
		RepeatEverySec:    900,
		UpdateCooldownSec: 120,
	}
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected ops_alerts.poll_interval_sec validation error")
	}
	if !strings.Contains(err.Error(), "ops_alerts.poll_interval_sec") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateOpsAlertsDisabledAllowsZeroValues(t *testing.T) {
	cfg := validConfig()
	cfg.OpsAlerts = OpsAlertsConfig{Enabled: false}
	if err := Validate(cfg); err != nil {
		t.Fatalf("expected valid config with ops_alerts disabled, got: %v", err)
	}
}
