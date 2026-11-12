my website

Database queries are defined in `internal/adapters/database/queries.sql` and
compiled by sqlc into `internal/adapters/database/dbquery`. Edit the SQL source,
then run `make sqlc`; do not edit generated Go query code. Schema and migration
DDL live alongside the queries in `.sql` files and are embedded at runtime.

Run `make sqlc-check` to verify generated code matches the SQL sources. The
Makefile pins sqlc to v1.30.0; generated code requires no sqlc runtime dependency.

R2 uses two distinct buckets with shared `R2_ACCOUNT_ID` (or `R2_ENDPOINT`),
`R2_ACCESS_KEY_ID`, and `R2_SECRET_ACCESS_KEY` credentials:

| Configuration | Contents |
| --- | --- |
| `R2_PHOTOS_BUCKET` + `R2_PHOTOS_PUBLIC_URL` | `photos/`, gallery backup `catalog.json` and `catalog/` |
| `R2_BLOG_BUCKET` + `R2_BLOG_PUBLIC_URL` | `images/`, `posts/`, blog backup `catalog.json` and `catalog/` |

Both bucket names and public URLs are required. These replace `R2_BUCKET` and
`R2_PUBLIC_URL`. Blog image uploads return the blog public URL; gallery uploads
return the photos public URL. New blog image keys use `images/<uuid>.<ext>`; gallery image keys use
`photos/<uuid>.<ext>`.

Changing configuration does not copy existing objects or rewrite saved image
URLs. When moving from one bucket, preserve the current SQLite database and
migrate existing objects and backup catalogs before switching storage. A fresh
SQLite database restores gallery records from the photos catalog and post
records from the blog catalog. Full post backups include drafts.

Image delivery preserves the original uploaded bytes. New image objects under
`photos/` and `images/` receive `Cache-Control: public, max-age=31536000, immutable`;
photo metadata updates retain this policy. Post backups and catalogs use
`no-store`. Uploads also save image dimensions, which the UI uses to reserve
space. The gallery eagerly loads its first row and lazy-loads later rows;
single photos load eagerly with high priority, and blog images near the start
of the body load eagerly while later images load lazily. Images decode
asynchronously.

Existing images remain compatible, but their stored dimensions and cache headers
are not automatically backfilled. Serve both buckets through their custom
Cloudflare domains for CDN caching. Smart Tiered Cache is a separate Cloudflare
zone setting; it is not configured by this repository.

Writes now commit to SQLite and immediately flush their queued records to R2
before the API reports success. Reads continue using SQLite during the upload.
Failed or interrupted writes return an error; they may already exist locally
or remotely, so reload before retrying. Their local queue is retried on the next
write and by a once-per-minute recovery/cleanup worker. An unacknowledged write
can be lost if the container disappears before the retry succeeds.

Fly and development Compose enable `EPHEMERAL_DATABASE=true`: every process
start creates a fresh SQLite file beneath `DATABASE_PATH`'s parent directory
and restores records from R2 before serving traffic. No database volume is
required. Suspend/resume keeps the running process; it does not perform a new
restore. A cold start requires R2 to be available. Local development defaults to
reusing `DATABASE_PATH` unless ephemeral mode is enabled.

Run a single application Machine: separate SQLite caches do not replicate live
updates between Machines. Use `fly deploy --ha=false` and keep the app scaled
to one Machine (`fly scale count 1`). Keep the rolling deployment strategy.

For the first transition off the old volume, stop editing and ensure the old
application's backup queue has drained before deploying. Keep the existing Fly
volume until the restored content has been verified; removing the mount from
the configuration does not delete that volume. No live volume is deleted by
these source changes.
