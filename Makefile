DEV_EXEC := mise exec --

.PHONY: build dev install frontend test test-ui vendor tidy vendor-check

build: frontend
	mkdir -p bin
	$(DEV_EXEC) go build -mod=vendor -o bin/website ./cmd/website

dev: frontend
	$(DEV_EXEC) go run -mod=vendor ./internal/testutil/cmd/testserver

install:
	mise install
	$(DEV_EXEC) npm --prefix ui ci --include=dev

frontend:
	$(DEV_EXEC) npm --prefix ui ci --include=dev
	$(DEV_EXEC) npm --prefix ui run build

test:
	$(DEV_EXEC) go test -mod=vendor -race ./...
	$(DEV_EXEC) npm --prefix ui run typecheck

# Run after changing dependencies; commit vendor/ together with go.mod and go.sum.
vendor:
	$(DEV_EXEC) go mod vendor

tidy:
	$(DEV_EXEC) go mod tidy
	$(DEV_EXEC) go mod vendor

# Regenerate separately so checking never changes the working vendor directory.
vendor-check:
	@set -eu; vendor_tmp=$$(mktemp -d); \
	trap 'rm -rf "$$vendor_tmp"' EXIT; \
	$(DEV_EXEC) go mod vendor -o "$$vendor_tmp/vendor"; \
	diff -qr vendor "$$vendor_tmp/vendor"

# One-time browser setup: cd ui && npx playwright install --with-deps chromium
test-ui:
	$(DEV_EXEC) npm --prefix ui ci --include=dev
	$(DEV_EXEC) npm --prefix ui run test:ui

.PHONY: sqlc sqlc-check
sqlc:
	$(DEV_EXEC) sqlc generate

sqlc-check:
	$(DEV_EXEC) sqlc diff
