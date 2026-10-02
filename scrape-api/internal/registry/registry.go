package registry

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/neokil/scraper/scrape-api/internal/model"
)

var (
	ErrWorkerNotFound    = errors.New("worker not found")
	ErrWorkerIncarnation = errors.New("worker incarnation changed")
	ErrNoAvailableWorker = errors.New("no available worker")
	ErrSessionNotFound   = errors.New("session not found")
	ErrPageNotFound      = errors.New("page not found")
	ErrSnapshotNotFound  = errors.New("snapshot not found")
	ErrInconsistentState = errors.New("inconsistent worker state")
)

type Registry struct {
	mu        sync.RWMutex
	workers   map[string]model.Worker
	sessions  map[string]model.Session
	pages     map[string]model.Page
	snapshots map[string]model.Snapshot
}

func New() *Registry {
	return &Registry{
		workers:   make(map[string]model.Worker),
		sessions:  make(map[string]model.Session),
		pages:     make(map[string]model.Page),
		snapshots: make(map[string]model.Snapshot),
	}
}

func (r *Registry) Register(input model.WorkerRegistration, now time.Time) (model.Worker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.validateWorkerStateLocked(input.WorkerID, input.Sessions, input.Snapshots); err != nil {
		return model.Worker{}, err
	}

	status := model.WorkerOnline
	createdAt := now
	if existing, ok := r.workers[input.WorkerID]; ok {
		createdAt = existing.CreatedAt
		if existing.Status == model.WorkerDraining && existing.IncarnationID == input.IncarnationID {
			status = model.WorkerDraining
		}
		if existing.IncarnationID != input.IncarnationID {
			r.removeWorkerSessionsLocked(input.WorkerID)
		}
	}
	worker := model.Worker{
		ID:             input.WorkerID,
		Hostname:       input.Hostname,
		Version:        input.Version,
		BaseURL:        input.BaseURL,
		VNCURL:         input.VNCURL,
		IncarnationID:  input.IncarnationID,
		Status:         status,
		Capacity:       input.Capacity,
		CPUPercent:     input.CPUPercent,
		MemoryBytes:    input.MemoryBytes,
		ActiveSessions: len(input.Sessions),
		LastHeartbeat:  now,
		CreatedAt:      createdAt,
	}
	r.workers[worker.ID] = worker
	r.reconcileWorkerLocked(worker.ID, input.Sessions, input.Snapshots)
	return r.workers[worker.ID], nil
}

func (r *Registry) Heartbeat(workerID string, heartbeat model.WorkerHeartbeat, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	worker, ok := r.workers[workerID]
	if !ok {
		return ErrWorkerNotFound
	}
	if worker.IncarnationID != heartbeat.IncarnationID {
		r.removeWorkerSessionsLocked(workerID)
		return ErrWorkerIncarnation
	}
	if err := r.validateWorkerStateLocked(workerID, heartbeat.Sessions, heartbeat.Snapshots); err != nil {
		return err
	}
	if worker.Status == model.WorkerOffline {
		worker.Status = model.WorkerOnline
	}
	worker.CPUPercent = heartbeat.CPUPercent
	worker.MemoryBytes = heartbeat.MemoryBytes
	worker.Capacity = heartbeat.Capacity
	worker.ActiveSessions = len(heartbeat.Sessions)
	worker.LastHeartbeat = now
	r.workers[workerID] = worker
	r.reconcileWorkerLocked(workerID, heartbeat.Sessions, heartbeat.Snapshots)
	return nil
}

func (r *Registry) validateWorkerStateLocked(workerID string, sessions []model.Session, snapshots []model.Snapshot) error {
	seenSessions := make(map[string]struct{}, len(sessions))
	seenPages := make(map[string]struct{})
	for _, session := range sessions {
		if session.ID == "" || (session.WorkerID != "" && session.WorkerID != workerID) {
			return fmt.Errorf("%w: invalid session identity", ErrInconsistentState)
		}
		if _, exists := seenSessions[session.ID]; exists {
			return fmt.Errorf("%w: duplicate session %s", ErrInconsistentState, session.ID)
		}
		seenSessions[session.ID] = struct{}{}
		if existing, exists := r.sessions[session.ID]; exists && existing.WorkerID != workerID {
			return fmt.Errorf("%w: session %s belongs to another worker", ErrInconsistentState, session.ID)
		}
		for _, page := range session.Pages {
			if page.ID == "" || (page.SessionID != "" && page.SessionID != session.ID) {
				return fmt.Errorf("%w: invalid page identity", ErrInconsistentState)
			}
			if _, exists := seenPages[page.ID]; exists {
				return fmt.Errorf("%w: duplicate page %s", ErrInconsistentState, page.ID)
			}
			seenPages[page.ID] = struct{}{}
			if existing, exists := r.pages[page.ID]; exists && existing.SessionID != session.ID {
				return fmt.Errorf("%w: page %s belongs to another session", ErrInconsistentState, page.ID)
			}
		}
	}

	seenSnapshots := make(map[string]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.ID == "" || (snapshot.WorkerID != "" && snapshot.WorkerID != workerID) {
			return fmt.Errorf("%w: invalid snapshot identity", ErrInconsistentState)
		}
		if _, exists := seenSnapshots[snapshot.ID]; exists {
			return fmt.Errorf("%w: duplicate snapshot %s", ErrInconsistentState, snapshot.ID)
		}
		seenSnapshots[snapshot.ID] = struct{}{}
		if existing, exists := r.snapshots[snapshot.ID]; exists && existing.WorkerID != workerID {
			return fmt.Errorf("%w: snapshot %s belongs to another worker", ErrInconsistentState, snapshot.ID)
		}
	}
	return nil
}

func (r *Registry) reconcileWorkerLocked(workerID string, sessions []model.Session, snapshots []model.Snapshot) {
	seenSessions := make(map[string]struct{}, len(sessions))
	seenPages := make(map[string]struct{})
	for _, session := range sessions {
		session.WorkerID = workerID
		session.VNCURL = "/v1/sessions/" + session.ID + "/vnc"
		seenSessions[session.ID] = struct{}{}
		r.sessions[session.ID] = session
		for _, page := range session.Pages {
			page.SessionID = session.ID
			r.pages[page.ID] = page
			seenPages[page.ID] = struct{}{}
		}
	}
	for id, session := range r.sessions {
		if session.WorkerID == workerID {
			if _, ok := seenSessions[id]; !ok {
				r.removeSessionLocked(id)
			}
		}
	}
	for id, page := range r.pages {
		session, ok := r.sessions[page.SessionID]
		if ok && session.WorkerID == workerID {
			if _, seen := seenPages[id]; !seen {
				delete(r.pages, id)
			}
		}
	}

	for id, snapshot := range r.snapshots {
		if snapshot.WorkerID == workerID {
			delete(r.snapshots, id)
		}
	}
	for _, snapshot := range snapshots {
		snapshot.WorkerID = workerID
		r.snapshots[snapshot.ID] = snapshot
	}

	worker := r.workers[workerID]
	worker.ActiveSessions = len(sessions)
	r.workers[workerID] = worker
}

func (r *Registry) ListWorkers() []model.Worker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.Worker, 0, len(r.workers))
	for _, worker := range r.workers {
		result = append(result, worker)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (r *Registry) GetWorker(id string) (model.Worker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	worker, ok := r.workers[id]
	if !ok {
		return model.Worker{}, ErrWorkerNotFound
	}
	return worker, nil
}

func (r *Registry) SetWorkerStatus(id string, status model.WorkerStatus) (model.Worker, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	worker, ok := r.workers[id]
	if !ok {
		return model.Worker{}, ErrWorkerNotFound
	}
	worker.Status = status
	r.workers[id] = worker
	return worker, nil
}

func (r *Registry) SelectWorker() (model.Worker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var candidates []model.Worker
	for _, worker := range r.workers {
		if worker.Status == model.WorkerOnline {
			candidates = append(candidates, worker)
		}
	}
	if len(candidates) == 0 {
		return model.Worker{}, ErrNoAvailableWorker
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ActiveSessions == candidates[j].ActiveSessions {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].ActiveSessions < candidates[j].ActiveSessions
	})
	return candidates[0], nil
}

func (r *Registry) AddSession(session model.Session) {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, existed := r.sessions[session.ID]
	r.sessions[session.ID] = session
	if !existed {
		worker := r.workers[session.WorkerID]
		worker.ActiveSessions++
		r.workers[worker.ID] = worker
	}
}

func (r *Registry) ListSessions(status, workerID string) []model.Session {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		if status != "" && string(session.Status) != status {
			continue
		}
		if workerID != "" && session.WorkerID != workerID {
			continue
		}
		result = append(result, session)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func (r *Registry) GetSession(id string) (model.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	session, ok := r.sessions[id]
	if !ok {
		return model.Session{}, ErrSessionNotFound
	}
	session.Pages = r.pagesForSessionLocked(id)
	return session, nil
}

func (r *Registry) DeleteSession(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removeSessionLocked(id)
}

func (r *Registry) removeSessionLocked(id string) {
	session, ok := r.sessions[id]
	if !ok {
		return
	}
	delete(r.sessions, id)
	for pageID, page := range r.pages {
		if page.SessionID == id {
			delete(r.pages, pageID)
		}
	}
	if worker, ok := r.workers[session.WorkerID]; ok && worker.ActiveSessions > 0 {
		worker.ActiveSessions--
		r.workers[worker.ID] = worker
	}
}

func (r *Registry) removeWorkerSessionsLocked(workerID string) {
	for id, session := range r.sessions {
		if session.WorkerID == workerID {
			r.removeSessionLocked(id)
		}
	}
	for id, snapshot := range r.snapshots {
		if snapshot.WorkerID == workerID {
			delete(r.snapshots, id)
		}
	}
}

func (r *Registry) AddPage(page model.Page) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pages[page.ID] = page
}

func (r *Registry) GetPage(id string) (model.Page, model.Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	page, ok := r.pages[id]
	if !ok {
		return model.Page{}, model.Session{}, ErrPageNotFound
	}
	session, ok := r.sessions[page.SessionID]
	if !ok {
		return model.Page{}, model.Session{}, ErrSessionNotFound
	}
	return page, session, nil
}

func (r *Registry) DeletePage(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.pages, id)
}

func (r *Registry) ListPages(sessionID string) []model.Page {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pagesForSessionLocked(sessionID)
}

func (r *Registry) pagesForSessionLocked(sessionID string) []model.Page {
	result := make([]model.Page, 0)
	for _, page := range r.pages {
		if page.SessionID == sessionID {
			result = append(result, page)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result
}

func (r *Registry) TouchSession(id string, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}
	if at.After(session.LastActivity) {
		session.LastActivity = at
		r.sessions[id] = session
	}
	return nil
}

func (r *Registry) AddSnapshot(snapshot model.Snapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshots[snapshot.ID] = snapshot
}

func (r *Registry) GetSnapshot(id string) (model.Snapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	snapshot, ok := r.snapshots[id]
	if !ok {
		return model.Snapshot{}, ErrSnapshotNotFound
	}
	return snapshot, nil
}

func (r *Registry) DeleteSnapshot(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.snapshots, id)
}

func (r *Registry) MarkOffline(now time.Time, timeout time.Duration) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var offline []string
	for id, worker := range r.workers {
		if worker.Status == model.WorkerOffline || now.Sub(worker.LastHeartbeat) <= timeout {
			continue
		}
		worker.Status = model.WorkerOffline
		worker.ActiveSessions = 0
		r.workers[id] = worker
		r.removeWorkerSessionsLocked(id)
		offline = append(offline, id)
	}
	sort.Strings(offline)
	return offline
}
