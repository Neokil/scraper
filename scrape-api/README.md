# scrape-api

The central Go control plane and the only client-facing entry point.

This folder owns:

- the Go module and `cmd/scrape-api` executable;
- public REST handlers and the internal worker client;
- in-memory worker/session/page/snapshot routing;
- JSON profile storage and validation;
- server-rendered dashboard and embedded noVNC client;
- public and worker OpenAPI contracts;
- the API container image.

Run its Go checks from this directory:

```bash
go test -race ./...
go vet ./...
go build ./cmd/scrape-api
```

OpenAPI validation and generated web assets use the Node tooling in this folder:

```bash
npm ci --ignore-scripts
npm run generate
npm run lint:openapi
```

The Dockerfile expects the repository root as its build context:

```bash
docker build -f scrape-api/Dockerfile .
```

See the root [README](../README.md) for runtime configuration and API examples.
