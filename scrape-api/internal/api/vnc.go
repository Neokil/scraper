package api

import (
	"bytes"
	"html/template"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/go-chi/chi/v5"
	webassets "github.com/neokil/scraper/scrape-api/web"
)

var vncTemplate = template.Must(template.ParseFS(webassets.Files, "templates/vnc.html"))

func (a *API) vncPage(w http.ResponseWriter, r *http.Request) {
	session, worker, ok := a.sessionWorker(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	a.recordActivity(r, session, worker)
	var body bytes.Buffer
	if err := vncTemplate.ExecuteTemplate(&body, "vnc.html", session.ID); err != nil {
		a.logger.Error("vnc template rendering failed", "error", err)
		http.Error(w, "template rendering failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body.Bytes())
}

func (a *API) vncWebsocket(w http.ResponseWriter, r *http.Request) {
	_, worker, ok := a.sessionWorker(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	target, err := url.Parse(worker.VNCURL)
	if err != nil {
		writeProblem(w, r, http.StatusBadGateway, "invalid_worker_vnc", err.Error())
		return
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.URL.Path = target.Path
		req.Host = target.Host
	}
	proxy.ErrorHandler = func(rw http.ResponseWriter, _ *http.Request, err error) {
		a.logger.Error("vnc proxy failed", "error", err, "worker_id", worker.ID)
		http.Error(rw, "VNC proxy unavailable", http.StatusBadGateway)
	}
	proxy.ServeHTTP(w, r)
}
