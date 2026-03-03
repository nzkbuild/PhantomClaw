package safety

import (
	"testing"
	"time"
)

func TestLearningWindowUsesConfiguredRange(t *testing.T) {
	mgr, err := NewManager("AUTO", SessionWindow{
		LearningStart: "03:30",
		LearningEnd:   "05:00",
	}, "UTC", nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	mgr.nowFn = func() time.Time {
		return time.Date(2026, 3, 3, 4, 0, 0, 0, time.UTC)
	}
	if !mgr.isLearningHours() {
		t.Fatal("expected 04:00 to be inside configured learning window")
	}
	if mgr.CurrentMode() != ModeObserve {
		t.Fatalf("mode=%s, want=%s during learning hours", mgr.CurrentMode(), ModeObserve)
	}

	mgr.nowFn = func() time.Time {
		return time.Date(2026, 3, 3, 6, 0, 0, 0, time.UTC)
	}
	if mgr.isLearningHours() {
		t.Fatal("expected 06:00 to be outside configured learning window")
	}
	if mgr.CurrentMode() != ModeAuto {
		t.Fatalf("mode=%s, want=%s outside learning hours", mgr.CurrentMode(), ModeAuto)
	}
}

func TestLearningWindowHandlesWrapAroundMidnight(t *testing.T) {
	mgr, err := NewManager("AUTO", SessionWindow{
		LearningStart: "22:00",
		LearningEnd:   "06:00",
	}, "UTC", nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	mgr.nowFn = func() time.Time {
		return time.Date(2026, 3, 3, 23, 30, 0, 0, time.UTC)
	}
	if !mgr.isLearningHours() {
		t.Fatal("expected 23:30 to be inside wrap-around learning window")
	}

	mgr.nowFn = func() time.Time {
		return time.Date(2026, 3, 4, 2, 0, 0, 0, time.UTC)
	}
	if !mgr.isLearningHours() {
		t.Fatal("expected 02:00 to be inside wrap-around learning window")
	}

	mgr.nowFn = func() time.Time {
		return time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	}
	if mgr.isLearningHours() {
		t.Fatal("expected 12:00 to be outside wrap-around learning window")
	}
}
