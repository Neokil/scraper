package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/neokil/scraper/scrape-api/internal/config"
	"github.com/neokil/scraper/scrape-api/internal/model"
	"github.com/neokil/scraper/scrape-api/internal/profiles"
	"github.com/neokil/scraper/scrape-api/internal/registry"
	"github.com/neokil/scraper/scrape-api/internal/workerclient"
)

type Dashboard interface {
	Register(chi.Router)
}

type API struct {
	cfg      config.Config
	registry *registry.Registry
	profiles *profiles.Repository
	workers  *workerclient.Client
	logger   *slog.Logger
}

func New(cfg config.Config, reg *registry.Registry, profileRepo *profiles.Repository, workers *workerclient.Client, logger *slog.Logger) *API {
	return &API{cfg: cfg, registry: reg, profiles: profileRepo, workers: workers, logger: logger}
}

func (a *API) Router(dashboard Dashboard) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(a.logRequests)

	r.Get("/healthz", a.health)
	r.Get("/readyz", a.ready)
	r.Get("/openapi/public.yaml", serveOpenAPI("public.openapi.yaml"))
	r.Get("/openapi/worker.yaml", serveOpenAPI("worker.openapi.yaml"))
	r.Post("/internal/workers/register", a.registerWorker)
	r.Post("/internal/workers/{workerId}/heartbeat", a.heartbeat)

	r.Route("/v1", func(r chi.Router) {
		r.Get("/workers", a.listWorkers)
		r.Get("/workers/{workerId}", a.getWorker)
		r.Post("/workers/{workerId}/drain", a.drainWorker)
		r.Post("/workers/{workerId}/resume", a.resumeWorker)

		r.Post("/sessions", a.createSession)
		r.Get("/sessions", a.listSessions)
		r.Get("/sessions/{sessionId}", a.getSession)
		r.Delete("/sessions/{sessionId}", a.deleteSession)
		r.Post("/sessions/{sessionId}/activity", a.sessionActivity)
		r.Post("/sessions/{sessionId}/pages", a.createPage)
		r.Get("/sessions/{sessionId}/pages", a.listPages)
		r.Get("/sessions/{sessionId}/events", a.sessionEvents)
		r.Get("/sessions/{sessionId}/vnc", a.vncPage)
		r.Get("/sessions/{sessionId}/vnc/websocket", a.vncWebsocket)

		r.Get("/pages/{pageId}", a.getPage)
		r.Delete("/pages/{pageId}", a.deletePage)
		for _, action := range []string{"navigate", "click", "type", "press", "select", "wait", "evaluate"} {
			r.Post("/pages/{pageId}/"+action, a.pageAction(action))
		}
		r.Get("/pages/{pageId}/html", a.pageStream("html"))
		r.Get("/pages/{pageId}/screenshot", a.pageStream("screenshot"))
		r.Get("/pages/{pageId}/url", a.pageStream("url"))
		r.Get("/pages/{pageId}/title", a.pageStream("title"))
		r.Get("/pages/{pageId}/events", a.pageEvents)
		r.Post("/pages/{pageId}/snapshots", a.createSnapshot)

		r.Get("/profiles", a.listProfiles)
		r.Post("/profiles", a.createProfile)
		r.Get("/profiles/{profileName}", a.getProfile)

		r.Get("/snapshots/{snapshotId}", a.getSnapshot)
		r.Delete("/snapshots/{snapshotId}", a.deleteSnapshot)
	})
	if dashboard != nil {
		dashboard.Register(r)
	}
	return r
}

func (a *API) StartMaintenance(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			for _, workerID := range a.registry.MarkOffline(now, a.cfg.WorkerOfflineTimeout) {
				a.logger.Warn("worker marked offline", "worker_id", workerID)
			}
		}
	}
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) ready(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ready",
		"online_workers": countOnline(a.registry.ListWorkers()),
	})
}

func countOnline(workers []model.Worker) int {
	count := 0
	for _, worker := range workers {
		if worker.Status == model.WorkerOnline {
			count++
		}
	}
	return count
}

func (a *API) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(wrapped, r)
		a.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.Status(),
			"bytes", wrapped.BytesWritten(),
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}
