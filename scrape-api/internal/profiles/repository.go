package profiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/neokil/scraper/scrape-api/internal/model"
)

var namePattern = regexp.MustCompile("^[a-z0-9][a-z0-9-]{0,62}$")

var (
	ErrNotFound = errors.New("profile not found")
	ErrExists   = errors.New("profile already exists")
)

type Repository struct {
	dir      string
	mu       sync.RWMutex
	profiles map[string]model.Profile
}

func New(dir string) (*Repository, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create profile directory: %w", err)
	}
	repo := &Repository{dir: dir, profiles: make(map[string]model.Profile)}
	if err := repo.load(); err != nil {
		return nil, err
	}
	if _, ok := repo.profiles["generic"]; !ok {
		generic := model.Profile{
			Name:        "generic",
			Description: "Default headed Chromium profile",
			Browser:     "chromium",
			Viewport:    &model.Viewport{Width: 1920, Height: 1080},
		}
		if err := repo.createLocked(generic); err != nil {
			return nil, err
		}
	}
	return repo, nil
}

func (r *Repository) load() error {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return fmt.Errorf("read profile directory: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(r.dir, entry.Name()))
		if err != nil {
			return fmt.Errorf("read profile %s: %w", entry.Name(), err)
		}
		var profile model.Profile
		if err := json.Unmarshal(data, &profile); err != nil {
			return fmt.Errorf("decode profile %s: %w", entry.Name(), err)
		}
		if err := Validate(profile); err != nil {
			return fmt.Errorf("validate profile %s: %w", entry.Name(), err)
		}
		if strings.TrimSuffix(entry.Name(), ".json") != profile.Name {
			return fmt.Errorf("profile filename %s does not match name %s", entry.Name(), profile.Name)
		}
		r.profiles[profile.Name] = profile
	}
	return nil
}

func (r *Repository) List() []model.Profile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]model.Profile, 0, len(r.profiles))
	for _, profile := range r.profiles {
		result = append(result, profile)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func (r *Repository) Get(name string) (model.Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	profile, ok := r.profiles[name]
	if !ok {
		return model.Profile{}, ErrNotFound
	}
	return profile, nil
}

func (r *Repository) Create(profile model.Profile) error {
	if err := Validate(profile); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.profiles[profile.Name]; exists {
		return ErrExists
	}
	return r.createLocked(profile)
}

func (r *Repository) createLocked(profile model.Profile) error {
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return fmt.Errorf("encode profile: %w", err)
	}
	data = append(data, byte(10))
	tmp, err := os.CreateTemp(r.dir, "."+profile.Name+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary profile: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o640); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	target := filepath.Join(r.dir, profile.Name+".json")
	if _, err := os.Stat(target); err == nil {
		return ErrExists
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("commit profile: %w", err)
	}
	r.profiles[profile.Name] = profile
	return nil
}

func Validate(profile model.Profile) error {
	if !namePattern.MatchString(profile.Name) {
		return fmt.Errorf("name must match %s", namePattern)
	}
	if profile.Browser != "chromium" {
		return errors.New("browser must be chromium")
	}
	if profile.Viewport != nil {
		if profile.Viewport.Width < 320 || profile.Viewport.Width > 7680 ||
			profile.Viewport.Height < 240 || profile.Viewport.Height > 4320 {
			return errors.New("viewport is outside supported bounds")
		}
	}
	if profile.DeviceScaleFactor < 0 || profile.DeviceScaleFactor > 4 {
		return errors.New("deviceScaleFactor must be between 0 and 4")
	}
	switch profile.ColorScheme {
	case "", "light", "dark", "no-preference":
	default:
		return errors.New("invalid colorScheme")
	}
	switch profile.ReducedMotion {
	case "", "reduce", "no-preference":
	default:
		return errors.New("invalid reducedMotion")
	}
	if profile.Proxy != nil && profile.Proxy.Server == "" {
		return errors.New("proxy.server is required")
	}
	return nil
}
