package profiles

import (
	"errors"
	"os"
	"testing"

	"github.com/neokil/scraper/scrape-api/internal/model"
)

func TestRepositoryCreatesGenericAndPersistsProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repo, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get("generic"); err != nil {
		t.Fatal(err)
	}
	profile := model.Profile{
		Name:     "german-shop",
		Browser:  "chromium",
		Locale:   "de-DE",
		Viewport: &model.Viewport{Width: 1280, Height: 720},
	}
	if err := repo.Create(profile); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir + "/german-shop.json"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.Get("german-shop")
	if err != nil {
		t.Fatal(err)
	}
	if got.Locale != "de-DE" {
		t.Fatalf("unexpected locale %q", got.Locale)
	}
}

func TestRepositoryRejectsUnsafeAndDuplicateProfiles(t *testing.T) {
	t.Parallel()
	repo, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(model.Profile{Name: "../escape", Browser: "chromium"}); err == nil {
		t.Fatal("unsafe name was accepted")
	}
	profile := model.Profile{Name: "safe", Browser: "chromium"}
	if err := repo.Create(profile); err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(profile); !errors.Is(err, ErrExists) {
		t.Fatalf("expected ErrExists, got %v", err)
	}
}
