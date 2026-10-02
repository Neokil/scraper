package apispec

import "embed"

// Files contains the public and internal OpenAPI source documents.
//
//go:embed *.yaml
var Files embed.FS
