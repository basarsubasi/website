# Website

Photo gallery and blog using Vue 3, TypeScript, Vite, Go net/http, SQLite through database/sql, and AWS SDK for Go v2. The gallery retains its original layout and CSS; see frontend/GALLERY-LICENSE.

SQLite is the source of truth for blog posts and image metadata. The API reads SQLite; the browser loads image bytes directly from R2 URLs. R2 also holds asynchronous copies of the metadata and complete blog posts for recovery.

## Structure

- cmd/website: HTTP server entry point.
- cmd/seed and cmd/unseed: fixture commands.
- internal/application.go: configuration, wiring, server and backup worker lifecycle.
- internal/domain and internal/usecase: models, business rules, and adapter interfaces.
- internal/adapters/http: API, admin sessions, and static UI.
- internal/adapters/database: SQLite records and durable backup queue.
- internal/adapters/r2: image storage, backup catalog, and post objects.
- internal/devseed: generated sample photos and posts.
- frontend: Vue and TypeScript.

The SQLite driver is CGO-free. SQL is parameterized; no query-generation step is required.

## Configuration

Copy .env.example to .env and fill in the admin password and R2 settings. Configuration precedence is CLI flags, process environment, the environment file, then defaults. Values are parsed without shell expansion.

| Setting | Value |
| --- | --- |
| ADMIN_PASSWORD | Required; at least 16 characters |
| DATABASE_PATH | Default data/website.db; override with -db |
| R2_ACCOUNT_ID | Required unless R2_ENDPOINT is supplied |
| R2_ENDPOINT | Optional endpoint override |
| R2_BUCKET | Required public bucket for photos and backups |
| R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY | Required S3 credentials with Object Read & Write access |
| R2_PUBLIC_URL | Required bucket custom domain or enabled development public URL |
| HTTP_ADDR | Default 127.0.0.1:8080; override with -addr |
| FRONTEND_DIR | Default frontend/dist; override with -frontend |
| COOKIE_SECURE | Default false; set true behind HTTPS |
| TRUSTED_PROXY_CIDRS | Optional comma-separated trusted reverse proxy CIDRs |
| ENV_FILE | Default .env; override with -env; an empty value disables file loading |

Missing required settings or an unusable SQLite file stops startup. A new database requires R2 to initialize/import its backup. An already initialized database can start, serve reads, and save post/metadata edits while R2 is unavailable. New image uploads still require R2.

**Everything in the bucket is public, including draft post backups.** Draft status hides posts only from the public website/API. Credentials remain on the Go server. The app does not configure bucket public access.

## Development

Requires Go 1.27.4, Node 24 or newer, and Linux or macOS.

~~~sh
cp .env.example .env
# Fill in .env.
npm ci --prefix frontend
go run ./cmd/website
~~~

In another terminal:

~~~sh
npm run dev --prefix frontend
~~~

Vite proxies API calls to Go. Open /admin to sign in. To build and serve the UI from Go:

~~~sh
make build
./bin/website
~~~

### Seed and cleanup

~~~sh
./scripts/seed.sh
./scripts/unseed.sh
# Select a different configuration/database:
./scripts/seed.sh -env .env.dev -db data/dev.db
./scripts/unseed.sh -env .env.dev -db data/dev.db
~~~

Seeding uploads 12 generated PNGs (six bw, six color) and creates four posts (three published, one draft). Fixed UUIDv4 IDs make reruns preserve existing edits. Cleanup removes only those fixed IDs, including edited or renamed fixtures; unrelated records are untouched. Use the same database and bucket for both commands.

Both commands commit local changes and attempt a backup/cleanup batch. R2 failures leave durable work for the server to retry. If the server's backup worker already holds the lock, it will deliver the queued changes. Missing fixtures are skipped on repeated cleanup. Orphan uploads outside the catalog are not scanned or deleted. Neither command runs automatically on startup.

### Docker development

~~~sh
docker compose -f compose.dev.yaml up --build
docker compose -f compose.dev.yaml exec backend sh scripts/seed.sh
docker compose -f compose.dev.yaml exec backend sh scripts/unseed.sh
~~~

Visit http://localhost:5173. Air rebuilds Go changes and Vite reloads frontend changes. The dev-data volume stores SQLite at /data/website.db; other named volumes cache dependencies/builds. Do not remove the data volume when you want to retain content and pending backups.

## Production

The production container defaults to `website -env ""`, disabling application-level .env loading. Supply configuration through injected environment variables, including Fly secrets. Local development still loads .env by default.

~~~sh
docker build -t website .
docker run --rm --env-file .env \
  -e COOKIE_SECURE=true -e DATABASE_PATH=/data/website.db \
  -v website-data:/data -p 8080:8080 website
~~~

The multi-stage image builds Vue and a CGO-disabled Go binary, then runs on debian:bookworm-slim with CA certificates and UID/GID 10001. Go serves the UI and API. Preserve the named data volume; bind mounts must be writable by UID/GID 10001. Use HTTPS at the reverse proxy; local HTTP testing needs COOKIE_SECURE=false.

Use one authoritative SQLite database per R2 bucket. Do not run independently restored writable databases against the same backup catalog. SQLite and the backup worker lock require a local filesystem; the file lock serializes the server and fixture commands sharing that database.

## Persistence, backup, and restore

SQLite stores complete post bodies, publication status, image metadata, object keys, and public image URLs. SQL indexes support filtering and pagination. A mutation and its backup operation commit in the same SQLite transaction. Public reads, including health checks and missing post lookups, never access R2.

The background worker checks for queued work every five seconds. It sends operations in order, acknowledges them only after R2 succeeds, and retries failures after restart. Remote I/O does not hold a SQLite transaction or block normal reads. Backups are asynchronous: a successful save means SQLite committed, not that the R2 copy is already current. Failures are logged as "R2 backup pending". Housekeeping retries queued R2 object deletions approximately once a minute.

R2 contains:

- catalog.json: image metadata, post summaries, object pointers, and pending deletions.
- `photos/<uuid>.<extension>`: image files. Fixtures use an additional unique path component.
- `posts/<post-uuid>/<revision-uuid>.json`: complete posts, including drafts.

ETag conditions protect catalog writes. Post updates upload a new object before switching the catalog pointer. Replaced/deleted objects are queued for cleanup. This is a **latest-state mirror**, not historical versions: deleting a post or image also deletes its remote copy after synchronization. A crash between upload and catalog commit can leave an orphan object. Public CDN caches may retain deleted files until expiration or purge.

On first initialization, the app imports the existing R2 catalog and post bodies into SQLite in one transaction, preserving IDs, timestamps, and image URLs. It records an initialization marker even for an empty catalog. Later restarts never import again, so deleted local content cannot reappear from a lagging backup. Stop the old R2-only writer before switching to this version.

To recover after losing a database, stop the server and start it with a **new** database path against the same bucket:

~~~sh
./bin/website -db data/restored.db
~~~

The new file imports the latest successfully synchronized R2 state. Changes still queued only in the lost database cannot be recovered from R2. Keep regular SQLite backups as well: stop all writers before copying the database and any WAL files, or use SQLite's backup tooling. Keep old files until you have verified the restored content. Never overwrite a running database.

## Authentication and validation

Admin sessions expire after six hours and are held in memory, so restarting requires signing in again. Login allows 20 attempts/client/minute and 200 admitted attempts/instance/minute. Already-blocked clients do not consume the global budget. Distributed attacks can still exhaust it; use edge controls for larger floods.

Client identity defaults to the direct connection IP. Behind a reverse proxy, set TRUSTED_PROXY_CIDRS to only the actual proxy networks and have the proxy overwrite or correctly append X-Forwarded-For. Otherwise clients behind that proxy share a login allowance. Never trust 0.0.0.0/0 or ::/0.

Uploads accept JPEG, PNG, and GIF files up to 25 MiB. Photo metadata includes name, country, city, year taken, and bw/color. Record, revision, and session IDs use UUIDv4.

## Checks

~~~sh
go test -race ./...
go vet ./...
npm run build --prefix frontend
~~~

Tests use temporary SQLite files, an in-memory object store, and a local S3 fixture. They do not modify real R2 content. Coverage includes public reads during R2 outages, CRUD, draft visibility, transactional imports, restart recovery, ordered backup retries, concurrent workers, authentication limits, and seed cleanup.
