.PHONY: build frontend test dev seed unseed dev-docker

build: frontend
	mkdir -p bin
	go build -o bin/website ./cmd/website

frontend:
	cd frontend && npm ci && npm run build

test:
	go test -race ./...
	cd frontend && npm run typecheck

dev:
	go run ./cmd/website

seed:
	sh scripts/seed.sh

unseed:
	sh scripts/unseed.sh

dev-docker:
	docker compose -f compose.dev.yaml up --build
