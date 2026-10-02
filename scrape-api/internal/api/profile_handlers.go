package api

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/neokil/scraper/scrape-api/internal/model"
	"github.com/neokil/scraper/scrape-api/internal/profiles"
)

func (a *API) listProfiles(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"profiles": a.profiles.List()})
}

func (a *API) getProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := a.profiles.Get(chi.URLParam(r, "profileName"))
	if err != nil {
		writeProblem(w, r, http.StatusNotFound, "profile_not_found", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, profile)
}

func (a *API) createProfile(w http.ResponseWriter, r *http.Request) {
	var profile model.Profile
	if err := decodeJSON(r, &profile); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	err := a.profiles.Create(profile)
	switch {
	case errors.Is(err, profiles.ErrExists):
		writeProblem(w, r, http.StatusConflict, "profile_exists", err.Error())
	case err != nil:
		writeProblem(w, r, http.StatusBadRequest, "invalid_profile", err.Error())
	default:
		writeJSON(w, http.StatusCreated, profile)
	}
}
