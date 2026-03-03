package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionStoreUsesConfiguredTimezoneForDayPartition(t *testing.T) {
	tmpDir := t.TempDir()
	fixedUTC := time.Date(2026, 3, 3, 20, 30, 0, 0, time.UTC) // 2026-03-04 04:30 in UTC+8

	store, err := NewSessionStoreWithClock(tmpDir, "Asia/Kuala_Lumpur", func() time.Time {
		return fixedUTC
	})
	if err != nil {
		t.Fatalf("NewSessionStoreWithClock: %v", err)
	}

	if err := store.Append(Turn{
		Pair:    "XAUUSD",
		Role:    "assistant",
		Content: "timezone test",
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	wantPath := filepath.Join(tmpDir, "2026-03-04.jsonl")
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected session file %s, stat error: %v", wantPath, err)
	}

	notExpectedPath := filepath.Join(tmpDir, "2026-03-03.jsonl")
	if _, err := os.Stat(notExpectedPath); err == nil {
		t.Fatalf("did not expect local-date session file %s", notExpectedPath)
	}
}

func TestSessionStoreFallsBackToLocalTimezoneOnInvalidLocation(t *testing.T) {
	store, err := NewSessionStore(t.TempDir(), "Invalid/Timezone")
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	if store.location == nil {
		t.Fatal("expected non-nil fallback location")
	}
}
