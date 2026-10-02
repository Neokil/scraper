package model

import "time"

type WorkerStatus string

const (
	WorkerOnline   WorkerStatus = "online"
	WorkerOffline  WorkerStatus = "offline"
	WorkerDraining WorkerStatus = "draining"
)

type Worker struct {
	ID             string       `json:"id"`
	Hostname       string       `json:"hostname"`
	Version        string       `json:"version"`
	BaseURL        string       `json:"-"`
	VNCURL         string       `json:"-"`
	IncarnationID  string       `json:"incarnation_id"`
	Status         WorkerStatus `json:"status"`
	Capacity       int          `json:"capacity"`
	CPUPercent     float64      `json:"cpu_percent"`
	MemoryBytes    uint64       `json:"memory_bytes"`
	ActiveSessions int          `json:"active_sessions"`
	LastHeartbeat  time.Time    `json:"last_heartbeat"`
	CreatedAt      time.Time    `json:"created_at"`
	Sessions       []Session    `json:"-"`
	Snapshots      []Snapshot   `json:"-"`
}

type SessionStatus string

const (
	SessionStarting SessionStatus = "starting"
	SessionRunning  SessionStatus = "running"
	SessionStopping SessionStatus = "stopping"
)

type Session struct {
	ID                 string        `json:"id"`
	WorkerID           string        `json:"worker"`
	Status             SessionStatus `json:"status"`
	Browser            string        `json:"browser"`
	Profile            string        `json:"profile"`
	CreatedAt          time.Time     `json:"created_at"`
	LastActivity       time.Time     `json:"last_activity"`
	IdleTimeoutSeconds int           `json:"idle_timeout_seconds"`
	Pages              []Page        `json:"pages,omitempty"`
	VNCURL             string        `json:"vnc_url,omitempty"`
}

type Page struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	URL          string    `json:"url"`
	Title        string    `json:"title"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	LastActivity time.Time `json:"last_activity"`
}

type Event struct {
	ID         string         `json:"id"`
	Timestamp  time.Time      `json:"timestamp"`
	SessionID  string         `json:"session,omitempty"`
	PageID     string         `json:"page,omitempty"`
	Type       string         `json:"type"`
	Message    string         `json:"message"`
	URL        string         `json:"url,omitempty"`
	SnapshotID string         `json:"snapshot_id,omitempty"`
	Context    map[string]any `json:"context,omitempty"`
}

type Snapshot struct {
	ID        string    `json:"id"`
	WorkerID  string    `json:"-"`
	SessionID string    `json:"session,omitempty"`
	PageID    string    `json:"page,omitempty"`
	Class     string    `json:"class"`
	Format    string    `json:"format"`
	CreatedAt time.Time `json:"created_at"`
	SizeBytes int64     `json:"size_bytes"`
}

type Viewport struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Geolocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Accuracy  float64 `json:"accuracy,omitempty"`
}

type Proxy struct {
	Server   string `json:"server"`
	Bypass   string `json:"bypass,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type Profile struct {
	Name              string            `json:"name"`
	Description       string            `json:"description,omitempty"`
	Browser           string            `json:"browser"`
	UserAgent         string            `json:"userAgent,omitempty"`
	Locale            string            `json:"locale,omitempty"`
	TimezoneID        string            `json:"timezoneId,omitempty"`
	Viewport          *Viewport         `json:"viewport,omitempty"`
	DeviceScaleFactor float64           `json:"deviceScaleFactor,omitempty"`
	IsMobile          bool              `json:"isMobile,omitempty"`
	HasTouch          bool              `json:"hasTouch,omitempty"`
	ColorScheme       string            `json:"colorScheme,omitempty"`
	ReducedMotion     string            `json:"reducedMotion,omitempty"`
	JavaScriptEnabled *bool             `json:"javaScriptEnabled,omitempty"`
	IgnoreHTTPSErrors bool              `json:"ignoreHTTPSErrors,omitempty"`
	AcceptDownloads   *bool             `json:"acceptDownloads,omitempty"`
	Geolocation       *Geolocation      `json:"geolocation,omitempty"`
	Permissions       []string          `json:"permissions,omitempty"`
	ExtraHTTPHeaders  map[string]string `json:"extraHTTPHeaders,omitempty"`
	Proxy             *Proxy            `json:"proxy,omitempty"`
}

type WorkerRegistration struct {
	WorkerID      string     `json:"worker_id"`
	Hostname      string     `json:"hostname"`
	Version       string     `json:"version"`
	BaseURL       string     `json:"base_url"`
	VNCURL        string     `json:"vnc_url"`
	IncarnationID string     `json:"incarnation_id"`
	Capacity      int        `json:"capacity"`
	CPUPercent    float64    `json:"cpu_percent"`
	MemoryBytes   uint64     `json:"memory_bytes"`
	Sessions      []Session  `json:"sessions"`
	Snapshots     []Snapshot `json:"snapshots,omitempty"`
}

type WorkerHeartbeat struct {
	IncarnationID string     `json:"incarnation_id"`
	CPUPercent    float64    `json:"cpu_percent"`
	MemoryBytes   uint64     `json:"memory_bytes"`
	Capacity      int        `json:"capacity"`
	Sessions      []Session  `json:"sessions"`
	Snapshots     []Snapshot `json:"snapshots,omitempty"`
}

type CreateSessionRequest struct {
	ID                 string  `json:"id,omitempty"`
	Browser            string  `json:"browser,omitempty"`
	Profile            string  `json:"profile,omitempty"`
	IdleTimeoutSeconds int     `json:"idle_timeout_seconds,omitempty"`
	ProfileSettings    Profile `json:"profile_settings,omitempty"`
}

type CreatePageRequest struct {
	ID  string `json:"id,omitempty"`
	URL string `json:"url,omitempty"`
}

type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Detail    string `json:"detail"`
	RequestID string `json:"request_id,omitempty"`
}
