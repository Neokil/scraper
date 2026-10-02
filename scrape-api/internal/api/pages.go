package api

import (
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/neokil/scraper/scrape-api/internal/model"
)

func (a *API) getPage(w http.ResponseWriter, r *http.Request) {
	page, session, worker, ok := a.pageWorker(w, r)
	if !ok {
		return
	}
	a.recordActivity(r, session, worker)
	writeJSON(w, http.StatusOK, page)
}

func (a *API) deletePage(w http.ResponseWriter, r *http.Request) {
	page, session, worker, ok := a.pageWorker(w, r)
	if !ok {
		return
	}
	if err := a.workers.JSON(r.Context(), worker, http.MethodDelete, "/internal/pages/"+url.PathEscape(page.ID), nil, nil); err != nil {
		a.workerProblem(w, r, err)
		return
	}
	a.registry.DeletePage(page.ID)
	_ = a.registry.TouchSession(session.ID, time.Now().UTC())
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) pageAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, session, worker, ok := a.pageWorker(w, r)
		if !ok {
			return
		}
		var input map[string]any
		if err := decodeJSON(r, &input); err != nil {
			writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		var output any
		path := "/internal/pages/" + url.PathEscape(page.ID) + "/" + action
		if err := a.workers.JSON(r.Context(), worker, http.MethodPost, path, input, &output); err != nil {
			a.workerProblem(w, r, err)
			return
		}
		if r.URL.Query().Get("monitoring") != "1" {
			_ = a.registry.TouchSession(session.ID, time.Now().UTC())
		}
		writeJSON(w, http.StatusOK, output)
	}
}

func (a *API) pageStream(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, session, worker, ok := a.pageWorker(w, r)
		if !ok {
			return
		}
		path := "/internal/pages/" + url.PathEscape(page.ID) + "/" + kind
		resp, err := a.workers.Stream(r.Context(), worker, path, r.URL.Query())
		if err != nil {
			a.workerProblem(w, r, err)
			return
		}
		defer resp.Body.Close()
		for _, header := range []string{"Content-Type", "Content-Length", "Content-Disposition", "Cache-Control"} {
			if value := resp.Header.Get(header); value != "" {
				w.Header().Set(header, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		if r.URL.Query().Get("monitoring") != "1" {
			_ = a.registry.TouchSession(session.ID, time.Now().UTC())
		}
	}
}

func (a *API) pageEvents(w http.ResponseWriter, r *http.Request) {
	page, session, worker, ok := a.pageWorker(w, r)
	if !ok {
		return
	}
	a.recordActivity(r, session, worker)
	a.proxyWorkerJSON(w, r, worker, "/internal/pages/"+url.PathEscape(page.ID)+"/events")
}

func (a *API) createSnapshot(w http.ResponseWriter, r *http.Request) {
	page, session, worker, ok := a.pageWorker(w, r)
	if !ok {
		return
	}
	var input map[string]any
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &input); err != nil {
			writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}
	var snapshot model.Snapshot
	path := "/internal/pages/" + url.PathEscape(page.ID) + "/snapshots"
	if err := a.workers.JSON(r.Context(), worker, http.MethodPost, path, input, &snapshot); err != nil {
		a.workerProblem(w, r, err)
		return
	}
	snapshot.WorkerID = worker.ID
	a.registry.AddSnapshot(snapshot)
	_ = a.registry.TouchSession(session.ID, time.Now().UTC())
	writeJSON(w, http.StatusCreated, snapshot)
}

func (a *API) getSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, worker, ok := a.snapshotWorker(w, r)
	if !ok {
		return
	}
	resp, err := a.workers.Stream(r.Context(), worker, "/internal/snapshots/"+url.PathEscape(snapshot.ID), nil)
	if err != nil {
		a.workerProblem(w, r, err)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if disposition := resp.Header.Get("Content-Disposition"); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (a *API) deleteSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, worker, ok := a.snapshotWorker(w, r)
	if !ok {
		return
	}
	if err := a.workers.JSON(r.Context(), worker, http.MethodDelete, "/internal/snapshots/"+url.PathEscape(snapshot.ID), nil, nil); err != nil {
		a.workerProblem(w, r, err)
		return
	}
	a.registry.DeleteSnapshot(snapshot.ID)
	w.WriteHeader(http.StatusNoContent)
}
