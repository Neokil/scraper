package dashboard

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/neokil/scraper/scrape-api/internal/model"
	"github.com/neokil/scraper/scrape-api/internal/profiles"
	"github.com/neokil/scraper/scrape-api/internal/registry"
	webassets "github.com/neokil/scraper/scrape-api/web"
)

type Dashboard struct {
	registry  *registry.Registry
	profiles  *profiles.Repository
	poll      time.Duration
	static    http.Handler
	templates *template.Template
}

type view struct {
	Title        string
	PollMS       int64
	Workers      []model.Worker
	Sessions     []model.Session
	Session      *model.Session
	Worker       *model.Worker
	Profiles     []model.Profile
	Error        string
	CurrentRoute string
}

func New(reg *registry.Registry, profileRepo *profiles.Repository, poll time.Duration) (*Dashboard, error) {
	staticFS, err := fs.Sub(webassets.Files, "static")
	if err != nil {
		return nil, err
	}
	templates, err := template.New("dashboard").Funcs(templateFunctions()).ParseFS(
		webassets.Files,
		"templates/index.html",
		"templates/session.html",
		"templates/profiles.html",
	)
	if err != nil {
		return nil, err
	}
	return &Dashboard{
		registry:  reg,
		profiles:  profileRepo,
		poll:      poll,
		static:    http.FileServer(http.FS(staticFS)),
		templates: templates,
	}, nil
}

func (d *Dashboard) Register(r chi.Router) {
	r.Handle("/static/*", http.StripPrefix("/static/", d.static))
	r.Get("/", d.index)
	r.Get("/sessions/{sessionId}", d.session)
	r.Get("/profiles", d.profileList)
}

func (d *Dashboard) index(w http.ResponseWriter, r *http.Request) {
	d.render(w, "index.html", view{
		Title:        "Browser infrastructure",
		PollMS:       d.poll.Milliseconds(),
		Workers:      d.registry.ListWorkers(),
		Sessions:     d.registry.ListSessions("", ""),
		CurrentRoute: r.URL.Path,
	})
}

func (d *Dashboard) session(w http.ResponseWriter, r *http.Request) {
	session, err := d.registry.GetSession(chi.URLParam(r, "sessionId"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	worker, err := d.registry.GetWorker(session.WorkerID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	d.render(w, "session.html", view{
		Title:        session.ID,
		PollMS:       d.poll.Milliseconds(),
		Session:      &session,
		Worker:       &worker,
		CurrentRoute: r.URL.Path,
	})
}

func (d *Dashboard) profileList(w http.ResponseWriter, r *http.Request) {
	d.render(w, "profiles.html", view{
		Title:        "Profiles",
		Profiles:     d.profiles.List(),
		CurrentRoute: r.URL.Path,
	})
}

func (d *Dashboard) render(w http.ResponseWriter, name string, data view) {
	var body bytes.Buffer
	if err := d.templates.ExecuteTemplate(&body, name, data); err != nil {
		http.Error(w, "template rendering failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body.Bytes())
}

func templateFunctions() template.FuncMap {
	return template.FuncMap{
		"time": func(value time.Time) string {
			if value.IsZero() {
				return "—"
			}
			return value.Local().Format("2006-01-02 15:04:05")
		},
		"memory": func(value uint64) string {
			return fmt.Sprintf("%.0f MiB", float64(value)/(1024*1024))
		},
		"deadline": func(session model.Session) string {
			deadline := session.LastActivity.Add(time.Duration(session.IdleTimeoutSeconds) * time.Second)
			return deadline.Local().Format("2006-01-02 15:04:05")
		},
	}
}
