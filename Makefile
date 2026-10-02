.PHONY: build test lint generate worker-install worker-build compose-up compose-down

build:
	cd scrape-api && go build ./...
	cd worker && npm run build

test:
	cd scrape-api && go test ./...
	cd worker && npm test

lint:
	cd scrape-api && go vet ./...
	cd worker && npm run lint

generate:
	cd scrape-api && npm run generate

worker-install:
	cd worker && npm ci

worker-build:
	cd worker && npm run build

compose-up:
	docker compose up --build

compose-down:
	docker compose down --remove-orphans
