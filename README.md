my website

Database queries are defined in `internal/adapters/database/queries.sql` and
compiled by sqlc into `internal/adapters/database/dbquery`. Edit the SQL source,
then run `make sqlc`; do not edit generated Go query code. Schema and migration
DDL live alongside the queries in `.sql` files and are embedded at runtime.

Run `make sqlc-check` to verify generated code matches the SQL sources. The
Makefile pins sqlc to v1.30.0; generated code requires no sqlc runtime dependency.
