.PHONY: build frontend test dev seed unseed dev-docker test-ui test-container vendor tidy vendor-check

build: frontend
	mkdir -p bin
	go build -mod=vendor -o bin/website ./cmd/website

frontend:
	cd frontend && npm ci && npm run build

test:
	go test -mod=vendor -race ./...
	cd frontend && npm run typecheck

dev:
	go run -mod=vendor ./cmd/website

# Run after changing dependencies; commit vendor/ together with go.mod and go.sum.
vendor:
	go mod vendor

tidy:
	go mod tidy
	go mod vendor

# Regenerate separately so checking never changes the working vendor directory.
vendor-check:
	@set -eu; vendor_tmp=$$(mktemp -d); \
	trap 'rm -rf "$$vendor_tmp"' EXIT; \
	go mod vendor -o "$$vendor_tmp/vendor"; \
	diff -qr vendor "$$vendor_tmp/vendor"

seed:
	sh scripts/seed.sh

unseed:
	sh scripts/unseed.sh

dev-docker:
	docker compose -f compose.dev.yaml up --build

# One-time browser setup: cd frontend && npx playwright install --with-deps chromium
test-ui:
	cd frontend && npm ci && npm run test:ui

test-container:
	python3 scripts/smoke-container.py

# Keep generated query code reproducible without adding sqlc to runtime dependencies.
SQLC = go run -mod=mod github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0

.PHONY: sqlc sqlc-check
sqlc:
	$(SQLC) generate

sqlc-check:
	$(SQLC) diff
