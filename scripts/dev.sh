#!/bin/sh
set -eu
mkdir -p .tmp/air
exec air -c .air.toml
