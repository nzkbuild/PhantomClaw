package skills

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/nzkbuild/PhantomClaw/internal/memory"
)

func resetTrackerForTest() {
	tracker = &cronTracker{active: make(map[string]struct{})}
}

func TestCronAddPersistsJob(t *testing.T) {
	resetTrackerForTest()

	dbPath := filepath.Join(t.TempDir(), "skills.db")
	db, err := memory.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	skill := CronAddSkill(CronDeps{DB: db})
	out, err := skill.Execute(json.RawMessage(`{"pair":"EURUSD","delay_minutes":1,"reason":"test persist"}`))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out == "" {
		t.Fatal("expected non-empty cron_add result")
	}

	jobs, err := db.ListPendingCronJobs()
	if err != nil {
		t.Fatalf("ListPendingCronJobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs)=%d, want=1", len(jobs))
	}
	if jobs[0].Pair != "EURUSD" || jobs[0].Reason != "test persist" || jobs[0].Status != "pending" {
		t.Fatalf("unexpected persisted job: %+v", jobs[0])
	}
}

func TestReplayPendingCronJobsFiresDueJobs(t *testing.T) {
	resetTrackerForTest()

	dbPath := filepath.Join(t.TempDir(), "skills.db")
	db, err := memory.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()

	jobID := "job-replay-1"
	if err := db.UpsertCronJob(jobID, "XAUUSD", "replay fire", time.Now().Add(20*time.Millisecond)); err != nil {
		t.Fatalf("UpsertCronJob: %v", err)
	}

	fired := make(chan struct{}, 1)
	err = ReplayPendingCronJobs(CronDeps{
		DB: db,
		OnWake: func(pair, reason string) {
			if pair == "XAUUSD" {
				fired <- struct{}{}
			}
		},
	})
	if err != nil {
		t.Fatalf("ReplayPendingCronJobs: %v", err)
	}

	select {
	case <-fired:
	case <-time.After(2 * time.Second):
		t.Fatal("expected replayed cron job to fire")
	}

	var status string
	if err := db.QueryRow("SELECT status FROM cron_jobs WHERE job_id = ?", jobID).Scan(&status); err != nil {
		t.Fatalf("read cron job status: %v", err)
	}
	if status != "fired" {
		t.Fatalf("status=%q, want=fired", status)
	}
}
