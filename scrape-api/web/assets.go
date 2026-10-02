package webassets

import "embed"

// Files contains source-separated templates and static assets embedded for deployment.
//
//go:embed templates/*.html static
var Files embed.FS
