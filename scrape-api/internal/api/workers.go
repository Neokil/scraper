package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/neokil/scraper/scrape-api/internal/model"
	"github.com/neokil/scraper/scrape-api/internal/registry"
)

func (a *API) registerWorker(w http.ResponseWriter, r *http.Request) {
	var input model.WorkerRegistration
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.WorkerID == "" || input.IncarnationID == "" || input.BaseURL == "" || input.VNCURL == "" {
		writeProblem(w, r, http.StatusBadRequest, "invalid_worker", "worker_id, incarnation_id, base_url, and vnc_url are required")
		return
	}
	if !validHTTPURL(input.BaseURL) || !validHTTPURL(input.VNCURL) {
		writeProblem(w, r, http.StatusBadRequest, "invalid_worker_url", "worker URLs must use http or https")
		return
	}
	worker, err := a.registry.Register(input, time.Now().UTC())
	if err != nil {
		a.logger.Warn("worker state rejected", "worker_id", input.WorkerID, "error", err)
		writeProblem(w, r, http.StatusConflict, "inconsistent_worker_state", err.Error())
		return
	}
	a.logger.Info("worker registered", "worker_id", worker.ID, "incarnation_id", worker.IncarnationID)
	writeJSON(w, http.StatusOK, worker)
}

func (a *API) heartbeat(w http.ResponseWriter, r *http.Request) {
	var input model.WorkerHeartbeat
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	err := a.registry.Heartbeat(chi.URLParam(r, "workerId"), input, time.Now().UTC())
	switch {
	case errors.Is(err, registry.ErrWorkerNotFound):
		writeProblem(w, r, http.StatusNotFound, "unknown_worker", "worker must register")
	case errors.Is(err, registry.ErrWorkerIncarnation):
		writeProblem(w, r, http.StatusConflict, "worker_incarnation_changed", "worker must register")
	case errors.Is(err, registry.ErrInconsistentState):
		a.logger.Warn("worker state rejected", "worker_id", chi.URLParam(r, "workerId"), "error", err)
		writeProblem(w, r, http.StatusConflict, "inconsistent_worker_state", err.Error())
	case err != nil:
		writeProblem(w, r, http.StatusInternalServerError, "heartbeat_failed", err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *API) listWorkers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"workers": a.registry.ListWorkers()})
}

func (a *API) getWorker(w http.ResponseWriter, r *http.Request) {
	worker, err := a.registry.GetWorker(chi.URLParam(r, "workerId"))
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "worker_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, worker)
}

func (a *API) drainWorker(w http.ResponseWriter, r *http.Request) {
	a.setWorkerStatus(w, r, model.WorkerDraining)
}

func (a *API) resumeWorker(w http.ResponseWriter, r *http.Request) {
	worker, err := a.registry.GetWorker(chi.URLParam(r, "workerId"))
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "worker_not_found", err.Error())
		return
	}
	if time.Since(worker.LastHeartbeat) > a.cfg.WorkerOfflineTimeout {
		writeProblem(w, r, http.StatusConflict, "worker_offline", "offline worker cannot be resumed")
		return
	}
	a.setWorkerStatus(w, r, model.WorkerOnline)
}

func (a *API) setWorkerStatus(w http.ResponseWriter, r *http.Request, status model.WorkerStatus) {
	worker, err := a.registry.SetWorkerStatus(chi.URLParam(r, "workerId"), status)
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "worker_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, worker)
}
