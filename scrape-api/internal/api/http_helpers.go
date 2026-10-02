package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/neokil/scraper/scrape-api/internal/model"
	"github.com/neokil/scraper/scrape-api/internal/workerclient"
)

func (a *API) proxyWorkerJSON(w http.ResponseWriter, r *http.Request, worker model.Worker, path string) {
	var output any
	if err := a.workers.JSON(r.Context(), worker, http.MethodGet, path, nil, &output); err != nil {
		a.workerProblem(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, output)
}

func (a *API) sessionWorker(w http.ResponseWriter, r *http.Request, sessionID string) (model.Session, model.Worker, bool) {
	session, err := a.registry.GetSession(sessionID)
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "session_not_found", err.Error())
		return model.Session{}, model.Worker{}, false
	}
	worker, err := a.registry.GetWorker(session.WorkerID)
	if err != nil || worker.Status == model.WorkerOffline {
		writeProblem(w, r, http.StatusServiceUnavailable, "worker_unavailable", "session worker is unavailable")
		return model.Session{}, model.Worker{}, false
	}
	return session, worker, true
}

func (a *API) pageWorker(w http.ResponseWriter, r *http.Request) (model.Page, model.Session, model.Worker, bool) {
	page, session, err := a.registry.GetPage(chi.URLParam(r, "pageId"))
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "page_not_found", err.Error())
		return model.Page{}, model.Session{}, model.Worker{}, false
	}
	worker, err := a.registry.GetWorker(session.WorkerID)
	if err != nil || worker.Status == model.WorkerOffline {
		writeProblem(w, r, http.StatusServiceUnavailable, "worker_unavailable", "page worker is unavailable")
		return model.Page{}, model.Session{}, model.Worker{}, false
	}
	return page, session, worker, true
}

func (a *API) snapshotWorker(w http.ResponseWriter, r *http.Request) (model.Snapshot, model.Worker, bool) {
	snapshot, err := a.registry.GetSnapshot(chi.URLParam(r, "snapshotId"))
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "snapshot_not_found", err.Error())
		return model.Snapshot{}, model.Worker{}, false
	}
	worker, err := a.registry.GetWorker(snapshot.WorkerID)
	if err != nil || worker.Status == model.WorkerOffline {
		writeProblem(w, r, http.StatusServiceUnavailable, "worker_unavailable", "snapshot worker is unavailable")
		return model.Snapshot{}, model.Worker{}, false
	}
	return snapshot, worker, true
}

func (a *API) workerProblem(w http.ResponseWriter, r *http.Request, err error) {
	var workerErr *workerclient.Error
	if errors.As(err, &workerErr) {
		status := http.StatusBadGateway
		if workerErr.Status >= 400 && workerErr.Status < 500 || workerErr.Status == http.StatusGatewayTimeout {
			status = workerErr.Status
		}
		code := "worker_error"
		detail := strings.TrimSpace(workerErr.Body)
		var problem model.Problem
		if json.Unmarshal([]byte(workerErr.Body), &problem) == nil {
			if problem.Code != "" {
				code = problem.Code
			}
			if problem.Detail != "" {
				detail = problem.Detail
			}
		}
		writeProblem(w, r, status, code, detail)
		return
	}
	writeProblem(w, r, http.StatusBadGateway, "worker_unavailable", err.Error())
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func decodeJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return errors.New("request body is required")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	title := strings.ReplaceAll(code, "_", " ")
	if len(title) > 0 {
		title = strings.ToUpper(title[:1]) + title[1:]
	}
	w.Header().Set("Content-Type", "application/problem+json")
	writeJSON(w, status, model.Problem{
		Type:      "https://scraper.local/problems/" + code,
		Title:     title,
		Status:    status,
		Code:      code,
		Detail:    detail,
		RequestID: middleware.GetReqID(r.Context()),
	})
}
