package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestBannerHeader(t *testing.T) {
	var buf bytes.Buffer
	b := NewBanner(&buf)
	b.Header("4.0.0")
	out := buf.String()
	if !strings.Contains(out, "PhantomClaw v4.0.0") {
		t.Errorf("header missing version: %q", out)
	}
}

func TestBannerStep(t *testing.T) {
	cases := []struct {
		status string
		icon   string
	}{
		{StatusOK, "✓"},
		{StatusWarn, "⚠"},
		{StatusFail, "✗"},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		b := NewBanner(&buf)
		b.Step("Test", tc.status, "detail")
		out := buf.String()
		if !strings.Contains(out, tc.icon) {
			t.Errorf("Step(%q) missing icon %s: %q", tc.status, tc.icon, out)
		}
		if !strings.Contains(out, "detail") {
			t.Errorf("Step(%q) missing detail: %q", tc.status, out)
		}
	}
}

func TestBannerReady(t *testing.T) {
	var buf bytes.Buffer
	b := NewBanner(&buf)
	b.Ready("All good")
	out := buf.String()
	if !strings.Contains(out, "All good") {
		t.Errorf("ready message missing: %q", out)
	}
}
