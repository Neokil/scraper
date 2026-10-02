FROM node:22-bookworm-slim AS web-build
WORKDIR /src/scrape-api
COPY scrape-api/package.json scrape-api/package-lock.json ./
COPY scrape-api/scripts/vendor-novnc.mjs ./scripts/vendor-novnc.mjs
RUN npm ci --ignore-scripts
RUN node scripts/vendor-novnc.mjs

FROM golang:1.24-bookworm AS go-build
WORKDIR /src/scrape-api
COPY scrape-api/go.mod scrape-api/go.sum ./
RUN go mod download
COPY scrape-api/api ./api
COPY scrape-api/cmd ./cmd
COPY scrape-api/internal ./internal
COPY scrape-api/profiles ./profiles
COPY scrape-api/web ./web
COPY --from=web-build /src/scrape-api/web/static/vendor ./web/static/vendor
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/scrape-api ./cmd/scrape-api

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
RUN useradd --system --uid 10001 --create-home scraper \
    && install -d -o scraper -g scraper /var/lib/scraper/profiles
COPY --from=go-build /out/scrape-api /usr/local/bin/scrape-api
COPY --from=go-build --chown=scraper:scraper /src/scrape-api/profiles /var/lib/scraper/profiles
USER scraper
ENV SERVER_ADDRESS=:8080
ENV PROFILES_DIR=/var/lib/scraper/profiles
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --retries=12 CMD ["scrape-api", "healthcheck"]
ENTRYPOINT ["scrape-api"]
