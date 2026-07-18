#!/bin/sh
# Seeds a fresh (empty) /data volume from a bind-mounted db, then hands off to
# the server. Never touches an already-populated volume. The seed file is a
# runtime bind mount, not an image layer - see .dockerignore for why *.sqlite3
# is never COPY'd into the build.
set -eu

seed="${ACOUSTIC_SEED_DB_PATH:-/seed/acousticdna.sqlite3}"

if [ ! -f "$ACOUSTIC_DB_PATH" ] && [ -f "$seed" ]; then
	if head -c 16 "$seed" | grep -q "SQLite format 3"; then
		echo "acousticdna: seeding empty volume at $ACOUSTIC_DB_PATH from $seed" >&2
		cp "$seed" "$ACOUSTIC_DB_PATH"
	else
		echo "acousticdna: $seed is not a real sqlite db (git-lfs pointer?) - run 'git lfs pull'; starting with an empty db" >&2
	fi
fi

exec /app/server "$@"
