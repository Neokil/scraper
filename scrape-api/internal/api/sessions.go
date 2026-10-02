package api

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/neokil/scraper/scrape-api/internal/id"
	"github.com/neokil/scraper/scrape-api/internal/model"
)

func (a *API) createSession(w http.ResponseWriter, r *http.Request) {
	var input model.CreateSessionRequest
	if err := decodeJSON(r, &input); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if input.Browser == "" {
		input.Browser = "chromium"
	}
	if input.Browser != "chromium" {
		writeProblem(w, r, http.StatusBadRequest, "unsupported_browser", "only chromium is supported")
		return
	}
	if input.Profile == "" {
		input.Profile = "generic"
	}
	profile, err := a.profiles.Get(input.Profile)
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "profile_not_found", err.Error())
		return
	}
	if input.IdleTimeoutSeconds == 0 {
		input.IdleTimeoutSeconds = int(a.cfg.DefaultIdleTimeout.Seconds())
	}
	if input.IdleTimeoutSeconds < 1 || input.IdleTimeoutSeconds > int((7*24*time.Hour).Seconds()) {
		writeProblem(w, r, http.StatusBadRequest, "invalid_idle_timeout", "idle_timeout_seconds must be between 1 and 604800")
		return
	}
	worker, err := a.registry.SelectWorker()
	if err != nil {
		writeProblem(w, r, http.StatusServiceUnavailable, "no_available_worker", err.Error())
		return
	}
	input.ID = id.New("sess")
	input.ProfileSettings = profile
	var session model.Session
	if err := a.workers.JSON(r.Context(), worker, http.MethodPost, "/internal/sessions", input, &session); err != nil {
		a.workerProblem(w, r, err)
		return
	}
	session.ID = input.ID
	session.WorkerID = worker.ID
	session.Profile = input.Profile
	session.Browser = input.Browser
	session.VNCURL = "/v1/sessions/" + session.ID + "/vnc"
	a.registry.AddSession(session)
	writeJSON(w, http.StatusCreated, session)
}

func (a *API) listSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": a.registry.ListSessions(r.URL.Query().Get("status"), r.URL.Query().Get("worker")),
	})
}

func (a *API) getSession(w http.ResponseWriter, r *http.Request) {
	session, worker, ok := a.sessionWorker(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	a.recordActivity(r, session, worker)
	writeJSON(w, http.StatusOK, session)
}

func (a *API) deleteSession(w http.ResponseWriter, r *http.Request) {
	session, worker, ok := a.sessionWorker(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	if err := a.workers.JSON(r.Context(), worker, http.MethodDelete, "/internal/sessions/"+url.PathEscape(session.ID), nil, nil); err != nil {
		a.workerProblem(w, r, err)
		return
	}
	a.registry.DeleteSession(session.ID)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) sessionActivity(w http.ResponseWriter, r *http.Request) {
	session, worker, ok := a.sessionWorker(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	now := time.Now().UTC()
	_ = a.registry.TouchSession(session.ID, now)
	_ = a.workers.JSON(r.Context(), worker, http.MethodPost, "/internal/sessions/"+url.PathEscape(session.ID)+"/activity", map[string]time.Time{"at": now}, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) createPage(w http.ResponseWriter, r *http.Request) {
	session, worker, ok := a.sessionWorker(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	var input model.CreatePageRequest
	if err := decodeJSON(r, &input); err != nil && !errors.Is(err, io.EOF) {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	input.ID = id.New("page")
	var page model.Page
	path := "/internal/sessions/" + url.PathEscape(session.ID) + "/pages"
	if err := a.workers.JSON(r.Context(), worker, http.MethodPost, path, input, &page); err != nil {
		a.workerProblem(w, r, err)
		return
	}
	page.ID = input.ID
	page.SessionID = session.ID
	a.registry.AddPage(page)
	_ = a.registry.TouchSession(session.ID, time.Now().UTC())
	writeJSON(w, http.StatusCreated, page)
}

func (a *API) listPages(w http.ResponseWriter, r *http.Request) {
	session, worker, ok := a.sessionWorker(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	a.recordActivity(r, session, worker)
	writeJSON(w, http.StatusOK, map[string]any{"pages": a.registry.ListPages(session.ID)})
}

func (a *API) sessionEvents(w http.ResponseWriter, r *http.Request) {
	session, worker, ok := a.sessionWorker(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	a.recordActivity(r, session, worker)
	a.proxyWorkerJSON(w, r, worker, "/internal/sessions/"+url.PathEscape(session.ID)+"/events")
}

func (a *API) recordActivity(r *http.Request, session model.Session, worker model.Worker) {
	if r.URL.Query().Get("monitoring") == "1" {
		return
	}
	now := time.Now().UTC()
	_ = a.registry.TouchSession(session.ID, now)
	_ = a.workers.JSON(r.Context(), worker, http.MethodPost, "/internal/sessions/"+url.PathEscape(session.ID)+"/activity", map[string]time.Time{"at": now}, nil)
}
