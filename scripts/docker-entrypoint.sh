#!/bin/sh
set -eu

# A mounted volume hides the ownership set during the image build.
if [ "$(id -u)" = "0" ]; then
    mkdir -p /data
    chown 10001:10001 /data
    exec gosu 10001:10001 /usr/local/bin/website "$@"
fi

exec /usr/local/bin/website "$@"
