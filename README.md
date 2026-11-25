# Website

With mise installed, run `mise trust` on your first checkout, then `make install` from the repository root. This installs the Go, Node.js, and sqlc versions in `mise.toml` and the UI dependencies from `ui/package-lock.json`.

Run `make dev` to build the UI and start the local test server at `http://127.0.0.1:18080` using temporary SQLite and in-memory object storage. Sign in at `/admin` with `ui-test-password-only`. Stop it with Ctrl+C. Each launch starts with empty data.

Gallery and blog are Go modules under `internal/module/`. Each owns its domain types, use cases, and SQLite adapter. `internal/application/` wires both modules into the HTTP server in `internal/handler/adapter/http/`. They share the SQLite connection and R2 backup queue in `internal/platform/`.

SQLite applies the numbered SQL files in `database/migrations/` at startup. Query sources live in `database/queries/gallery/`, `database/queries/blog/`, and `database/queries/storage/`. Run `make sqlc` after a query change to regenerate `gen/sqlc/`, and `make sqlc-check` to verify it.

The Vue app lives in `ui/`. Feature pages are under `ui/src/pages/`, and browser tests are under `ui/tests/`. Cross-module storage and recovery tests live in `tests/integration/`.
