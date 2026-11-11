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
| RESTORE_TIMEOUT | First-start database initialization deadline; default 5m; accepts Go durations such as 30s or 10m |
| R2_ACCOUNT_ID | Required unless R2_ENDPOINT is supplied |
| R2_ENDPOINT | Optional endpoint override |
| R2_BUCKET | Required public bucket for photos and backups |
| R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY | Required S3 credentials with Object Read & Write access |
| R2_PUBLIC_URL | Required bucket custom domain or enabled development public URL |
| HTTP_ADDR | Default 127.0.0.1:8080; override with -addr |
| HTTP_MAX_REQUESTS | Maximum active handlers, including health checks and static files; default 32 |
| HTTP_MAX_UPLOADS | Maximum active image uploads, within the request limit; default 2 |
| HTTP_MAX_CONNECTIONS | Maximum accepted connections, including idle keep-alive connections; default 128 |
| FRONTEND_DIR | Default frontend/dist; override with -frontend |
| COOKIE_SECURE | Default false; set true behind HTTPS |
| TRUSTED_PROXY_CIDRS | Optional comma-separated trusted reverse proxy CIDRs |
| TRUST_FLY_PROXY | Default false; fly.toml enables Fly-Client-IP handling for private-network peers |
| ENV_FILE | Default .env; override with -env; an empty value disables file loading |

Missing required settings or an unusable SQLite file stops startup. A new database requires R2 to initialize/import its backup. An already initialized database can start, serve reads, and save post/metadata edits while R2 is unavailable. New image uploads still require R2.

Concurrency limits are per process and must be positive integers. Excess requests or uploads receive `503 Service Unavailable` with `Retry-After: 1` without waiting for a handler slot. At the connection limit, the server stops accepting until a connection closes; additional connections may wait in the OS backlog or time out. Upload admission happens after authentication and before reading multipart data. These are best-effort overload controls for the 256 MB Fly machine, not a hard cap on total goroutines or memory. Background workers and Go/library internals use additional goroutines.

**Everything in the bucket is public, including draft post backups.** Draft status hides posts only from the public website/API. Credentials remain on the Go server. The app does not configure bucket public access.

## Development

Requires Go 1.27.4, Node 24 or newer, and Linux or macOS.

Go dependencies are checked into `vendor/`. Builds and tests use these local
copies; Docker builds also copy them instead of downloading application modules.
The development image still downloads the Air tool during its build.

After changing Go dependencies:

~~~sh
make vendor       # Regenerate vendor/ from the current module dependencies.
make tidy         # Tidy go.mod/go.sum, then regenerate vendor/.
make vendor-check # Check vendor/ against a fresh copy without modifying it.
~~~

Commit `vendor/`, `go.mod`, and `go.sum` together. Dependency maintenance commands
may download modules; normal application builds do not need module downloads.

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

The multi-stage image builds Vue and a CGO-disabled Go binary, then runs on
`gcr.io/distroless/static-debian13:nonroot` with CA certificates. Go is the direct
entrypoint and runs as UID/GID 10001 from startup; the production image contains
no shell, package manager, or root startup script. Keeping the existing UID
preserves access to databases created by earlier images. Go serves the UI and API.

`/data` is owned by 10001 in the image, so fresh Docker named volumes inherit
writable permissions. Bind mounts and existing database files must already be
writable by UID/GID 10001. Fly's init assigns the mount directory to the configured
image user; see the [Fly init implementation](https://github.com/superfly/init-snapshot/blob/master/src/bin/init/main.rs).
Use HTTPS at the reverse proxy; local HTTP testing needs COOKIE_SECURE=false.

Use one authoritative SQLite database per R2 bucket. Do not run independently restored writable databases against the same backup catalog. SQLite and the backup worker lock require a local filesystem; the file lock serializes the server and fixture commands sharing that database.

### Fly.io

The production domain is `basarsubasi.com.tr`. The blog is served at `/` (also available at `/blog`), photos at `/gallery`, and the admin panel at `/admin`.

`fly.toml` configures HTTPS on port 8080, secure cookies, a persistent `/data`
volume, SQLite health checks, and graceful shutdown. The 256 MB Machine stops
when idle and starts again on incoming traffic. Pending R2 backups remain in
SQLite while stopped and resume after startup; admin sessions reset on restart.
First startup
gets a 310-second health-check grace period for the initial R2 import, with
`RESTORE_TIMEOUT=5m` and a seven-minute deployment wait. If you increase the
restore timeout, increase the Fly grace period and deployment wait as well.

Create the app, choose its region, and set the required secrets before deploying:

~~~sh
fly apps create YOUR_APP
# Set ADMIN_PASSWORD, R2_ACCOUNT_ID (or R2_ENDPOINT), R2_BUCKET,
# R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, and R2_PUBLIC_URL using Fly secrets.
# Import a file containing ONLY those KEY=VALUE settings (keep it outside Git):
fly secrets import --app YOUR_APP < /secure/path/fly-secrets.env
./scripts/deploy-fly.sh YOUR_APP
fly checks list --app YOUR_APP
~~~

The primary region is `ams` (Amsterdam), configured in `fly.toml`.
The deployment script takes the app name and passes `--ha=false`
to avoid creating a spare Machine with an independent writable SQLite database.
The mount configuration provisions a 2 GB `website_data` volume on first deploy
and retains daily snapshots for 14 days. For an existing Fly app, verify it has
exactly one Machine and the correct data volume before deploying; `--ha=false`
does not remove existing Machines. Do not scale this app horizontally. Rolling
updates of the single Machine cause a brief interruption. Do not add a release
command for database initialization: Fly release Machines do not mount the volume.

`TRUST_FLY_PROXY=true` uses the validated `Fly-Client-IP` header for login limits
only when the transport peer has a private IP. It takes precedence over
`TRUSTED_PROXY_CIDRS`. Enable it only with Fly Proxy as the public entry point
and trusted workloads on the private network: a private address alone does not
authenticate a proxy. Missing or malformed headers fall back to the connection
IP. A CDN in front of Fly will be counted as the client; this configuration
assumes browsers connect directly to Fly. Same-origin protection remains enabled.

See [Fly app configuration](https://fly.io/docs/reference/configuration/),
[deployment options](https://fly.io/docs/flyctl/deploy/), and
[proxy headers](https://fly.io/docs/networking/request-headers/).

## Persistence, backup, and restore

SQLite stores complete post bodies, publication status, image metadata, object keys, and public image URLs. SQL indexes support filtering and pagination. A mutation and its backup operation commit in the same SQLite transaction. Public reads, including health checks and missing post lookups, never access R2.

The background worker checks for queued work every five seconds. It sends operations in order, acknowledges them only after R2 succeeds, and retries failures after restart. Remote I/O does not hold a SQLite transaction or block normal reads. Backups are asynchronous: a successful save means SQLite committed, not that the R2 copy is already current. Failures are logged as "R2 backup pending". Housekeeping retries queued R2 object deletions approximately once a minute.

R2 contains:

- catalog.json: image metadata, post summaries, object pointers, and pending deletions; large catalogs use a version-2 manifest referencing immutable parts under catalog/.
- `photos/<uuid>.<extension>`: image files. Fixtures use an additional unique path component.
- `posts/<post-uuid>/<revision-uuid>.json`: complete posts, including drafts.

ETag conditions protect catalog writes. Post updates upload a new object before switching the catalog pointer. Replaced/deleted objects are queued for cleanup. This is a **latest-state mirror**, not historical versions: deleting a post or image also deletes its remote copy after synchronization. A crash between upload and catalog commit can leave an orphan object. Public CDN caches may retain deleted files until expiration or purge.

On first initialization, the app imports the existing R2 catalog and post bodies into SQLite in one transaction, preserving IDs, timestamps, and image URLs. Post downloads use up to eight concurrent requests, each with a ten-second timeout, within `RESTORE_TIMEOUT`. A failed or canceled import leaves the database uninitialized so it can retry safely. It records an initialization marker even for an empty catalog. Later restarts never import again, so deleted local content cannot reappear from a lagging backup. Stop the old R2-only writer before switching to this version.

Public blog lists read a separate summary table, maintained transactionally with
the full post. Existing databases are backfilled automatically on startup.
New post edits limit excerpts to 2,000 characters; existing backups retain their
complete excerpts. Catalogs larger than 4 MiB split into bounded immutable parts,
so their total record size can exceed the former 16 MiB single-object limit.
The root manifest is updated conditionally only after every part is uploaded;
retired parts are cleaned through the existing durable deletion queue. Existing
version-1 catalogs remain readable, but older binaries cannot read version-2
manifests after a large catalog is upgraded. Do not roll back to an older reader
against an upgraded bucket.

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

### Browser tests

Requires Go, Node, and Chromium installed through Playwright:

~~~sh
cd frontend
npm ci
npx playwright install --with-deps chromium
cd ..
make test-ui
~~~

Playwright builds the production frontend and starts the real Go application on
`127.0.0.1:18080`. The test server uses a temporary SQLite database and a local
in-memory S3 service, overrides cloud settings with dummy values, and disables
.env loading. It refuses to reuse an existing server. Tests cover failed login,
session reload/logout, photo upload/display/edit/filter/delete, draft privacy,
publishing and editing posts, deep-link reloads, and invalid image uploads.
No real R2 bucket is accessed. Tests run with one Chromium worker and no automatic
retries. Failure screenshots and traces are saved under `frontend/test-results/`.
Use `npm run test:ui -- --grep "photo"` from `frontend/` to select a test.

### Production container smoke test

Requires Docker and Python 3:

~~~sh
make test-container
~~~

The script builds the production image from `Dockerfile` and a separate S3
fixture image from `internal/testutil/Dockerfile`, then
creates uniquely named containers, a private Docker network, and two disposable
volumes. It checks the non-root user, absent shell, read-only root filesystem,
frontend serving, health, secure cookie flags, post creation, image upload, actual
background backup delivery, restore into a fresh database, restart while R2 is
offline, and graceful shutdown. Secure cookies are asserted at the HTTP level;
the smoke client forwards the cookie explicitly because its local URL is HTTP.

All test containers, volumes, and image tags are removed on success or failure.
The production image does not contain the fixture binary. Both test suites are
local commands. Live Fly/R2 credentials, TLS, and edge routing still require a
staging deployment check.

### Scheduled dependency checks

GitHub Actions runs dependency security checks daily at 06:23 UTC, on pushes to
`main`, on pull requests, and manually from the Actions tab. `govulncheck` fails
for reachable Go vulnerabilities; `npm audit` fails for reported frontend
vulnerabilities, including development dependencies. Tool failures also fail
the workflow. These scans require no Fly or R2 credentials.

Go scanning uses `GOFLAGS=-mod=readonly` to resolve `go.mod`/`go.sum` instead of
the local vendor tree. Dependencies are still checked for vulnerabilities.
Pushes and PRs changing only `vendor/` do not trigger the security workflow;
the daily schedule still checks the current default branch.

Renovate runs daily at 06:47 UTC, or manually, and opens update PRs for Go, npm,
Docker images, GitHub Actions, and the scanner tools. Ordinary updates do not
fail the security workflow; they appear as PRs and in the Dependency Dashboard
issue. Renovate refreshes existing update PRs on subsequent runs; pending updates
do not fail the workflow. Updates are never automatically merged. `vendor/` is ignored during
dependency discovery, but Renovate automatically regenerates it alongside Go
module updates. Go module major upgrades can require manual import-path changes
and `make tidy`; Renovate does not support automatically vendoring those upgrades.
See [Renovate's Go support](https://docs.renovatebot.com/golang/).

To enable Renovate, add a repository Actions secret named `RENOVATE_TOKEN` in
GitHub Settings → Secrets and variables → Actions. Use a fine-grained PAT scoped
to this repository with Contents, Pull requests, Issues, Commit statuses, and
Workflows read/write, plus Dependabot alerts read access. Organization-owned
repositories also need Members read access. Alternatively, a classic PAT needs
`repo` (or `public_repo` for a public repository) and `workflow` scopes.
The job reports a clear failure if the secret is missing. See
[Renovate token permissions](https://docs.renovatebot.com/modules/platform/github/).

Commit and push these files to the default branch to activate the schedules.
Enable Actions email notifications (optionally failures only) in your GitHub
notification settings, and watch the repository's pull requests/issues for
update notifications. Scheduled-run notifications go to the schedule's creator
or the last person who changed its cron expression. GitHub may disable scheduled
workflows in inactive public repositories after 60 days; re-enable them in the
Actions tab if that happens. See
[workflow notifications](https://docs.github.com/en/actions/concepts/workflows-and-actions/notifications-for-workflow-runs).
