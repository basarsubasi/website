#!/bin/sh
set -eu

: "${1:?Usage: $0 APP}"

cd "$(dirname "$0")/.."
# SQLite has one authoritative writer. Never provision a standby with its own DB.
exec fly deploy --config fly.toml --app "$1" --ha=false
