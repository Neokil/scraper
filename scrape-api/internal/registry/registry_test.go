package registry

import (
	"errors"
	"testing"
	"time"

	"github.com/neokil/scraper/scrape-api/internal/model"
)

func TestLeastLoadedSchedulingAndDrain(t *testing.T) {
	t.Parallel()
	reg := New()
	now := time.Now().UTC()
	reg.Register(registration("worker-b", "boot-b", 2), now)
	reg.Register(registration("worker-a", "boot-a", 1), now)

	selected, err := reg.SelectWorker()
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "worker-a" {
		t.Fatalf("expected stable least-loaded worker-a, got %s", selected.ID)
	}
	reg.AddSession(model.Session{ID: "sess_new", WorkerID: "worker-a"})
	selected, err = reg.SelectWorker()
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "worker-b" {
		t.Fatalf("expected worker-b, got %s", selected.ID)
	}
	if _, err := reg.SetWorkerStatus("worker-b", model.WorkerDraining); err != nil {
		t.Fatal(err)
	}
	selected, err = reg.SelectWorker()
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != "worker-a" {
		t.Fatalf("draining worker was selected: %s", selected.ID)
	}
}

func TestNewIncarnationRemovesSessions(t *testing.T) {
	t.Parallel()
	reg := New()
	now := time.Now().UTC()
	input := registration("worker-a", "boot-1", 1)
	reg.Register(input, now)
	if _, err := reg.GetSession("sess_worker-a"); err != nil {
		t.Fatal(err)
	}

	input.IncarnationID = "boot-2"
	input.Sessions = nil
	reg.Register(input, now.Add(time.Second))
	if _, err := reg.GetSession("sess_worker-a"); err != ErrSessionNotFound {
		t.Fatalf("expected session removal, got %v", err)
	}
}

func TestHeartbeatReconcilesPages(t *testing.T) {
	t.Parallel()
	reg := New()
	now := time.Now().UTC()
	reg.Register(registration("worker-a", "boot-1", 1), now)
	err := reg.Heartbeat("worker-a", model.WorkerHeartbeat{
		IncarnationID: "boot-1",
		Capacity:      3,
		Sessions: []model.Session{{
			ID:       "sess_existing",
			WorkerID: "worker-a",
			Status:   model.SessionRunning,
			Pages:    []model.Page{{ID: "page-new", SessionID: "sess_existing"}},
		}},
	}, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reg.GetPage("page-old-worker-a"); err != ErrPageNotFound {
		t.Fatalf("expected stale page to be removed, got %v", err)
	}
	if _, _, err := reg.GetPage("page-new"); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterRejectsSessionOwnedByAnotherWorker(t *testing.T) {
	t.Parallel()
	reg := New()
	now := time.Now().UTC()
	reg.Register(registration("worker-a", "boot-a", 1), now)
	input := registration("worker-b", "boot-b", 1)
	input.Sessions[0].ID = "sess_worker-a"
	if _, err := reg.Register(input, now); !errors.Is(err, ErrInconsistentState) {
		t.Fatalf("expected inconsistent state, got %v", err)
	}
	if session, err := reg.GetSession("sess_worker-a"); err != nil || session.WorkerID != "worker-a" {
		t.Fatalf("existing routing changed: session=%+v err=%v", session, err)
	}
}

func registration(workerID, incarnation string, sessions int) model.WorkerRegistration {
	result := model.WorkerRegistration{
		WorkerID:      workerID,
		Hostname:      workerID,
		Version:       "test",
		BaseURL:       "http://" + workerID + ":8081",
		VNCURL:        "http://" + workerID + ":6080/websockify",
		IncarnationID: incarnation,
		Capacity:      3,
	}
	if sessions > 0 {
		result.Sessions = []model.Session{{
			ID:       "sess_" + workerID,
			WorkerID: workerID,
			Status:   model.SessionRunning,
			Pages:    []model.Page{{ID: "page-old-" + workerID, SessionID: "sess_" + workerID}},
		}}
	}
	return result
}
