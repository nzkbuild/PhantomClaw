package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

type fakeAlertSender struct {
	called bool
	text   string
}

func (f *fakeAlertSender) Send(ctx context.Context, text string) {
	f.called = true
	f.text = text
}

func TestMakeSessionAlertSenderCallsSend(t *testing.T) {
	logger := zap.NewNop().Sugar()
	sender := &fakeAlertSender{}

	cb := makeSessionAlertSender(sender, logger)
	cb(context.Background(), "hello alert")

	if !sender.called {
		t.Fatal("expected sender.Send to be called")
	}
	if sender.text != "hello alert" {
		t.Fatalf("sender text=%q, want=%q", sender.text, "hello alert")
	}
}

func TestMakeSessionAlertSenderHandlesNilSender(t *testing.T) {
	logger := zap.NewNop().Sugar()

	cb := makeSessionAlertSender(nil, logger)
	cb(context.Background(), "noop")
}

func TestMakeBridgeProbeWithAccountSnapshot(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"phantomclaw","version":"3.0.0","contract":"v3"}`))
	})
	mux.HandleFunc("/account", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Phantom-Bridge-Token"); got != "bridge-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"open_positions":2,"timestamp":"2026-03-03 21:30:00"}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	probe := makeBridgeProbe(ts.URL, "bridge-secret")
	result, err := probe(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.Service != "phantomclaw" || result.Version != "3.0.0" || result.Contract != "v3" {
		t.Fatalf("unexpected health result: %+v", result)
	}
	if !result.EAConnected || result.OpenPositions != 2 || result.EATimestamp == "" {
		t.Fatalf("unexpected account result: %+v", result)
	}
}

func TestMakeBridgeProbeWithoutAccountSnapshot(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","service":"phantomclaw","version":"3.0.0","contract":"v3"}`))
	})
	mux.HandleFunc("/account", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no account snapshot yet", http.StatusNotFound)
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	probe := makeBridgeProbe(ts.URL, "bridge-secret")
	result, err := probe(context.Background())
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if result.EAConnected {
		t.Fatalf("expected EAConnected=false when account is not found, got %+v", result)
	}
}
