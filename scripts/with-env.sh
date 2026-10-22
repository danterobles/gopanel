#!/bin/sh
# .env is a trusted local shell assignment file, never a source from a request.
set -eu
if [ -f .env ]; then
    set -a
    . ./.env
    set +a
fi
exec "$@"
