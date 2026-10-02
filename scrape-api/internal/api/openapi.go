package api

import (
	"net/http"

	apispec "github.com/neokil/scraper/scrape-api/api"
)

func serveOpenAPI(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := apispec.Files.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(data)
	}
}
