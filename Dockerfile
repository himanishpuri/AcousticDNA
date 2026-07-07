# syntax=docker/dockerfile:1

############################
# Stage 1 - Go builder
############################
FROM golang:1.25.5-bookworm AS builder

WORKDIR /src

# Dependency layer: cached unless go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Server: CGO-free static binary (glebarez pure-Go sqlite - KEEP invariant).
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/server ./cmd/server

# WASM fingerprint module (js/wasm build-tag path) + runtime glue, staged with
# the static web assets. Server and WASM are built from the same source tree,
# so createAddress (WASM) and IsValidHash (server) stay in lockstep.
RUN GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" \
      -o web/public/fingerprint.wasm ./cmd/wasm \
 && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/public/wasm_exec.js \
 && cp -r web/public /out/web-public

############################
# Stage 2 - runtime
############################
FROM debian:bookworm-slim AS runtime

# Bump to match the yt-dlp version pinned by ws5.
ARG YTDLP_VERSION=2025.06.30
# Auto-populated by BuildKit from the target platform (amd64 or arm64).
ARG TARGETARCH

# ffmpeg (decode/resample), yt-dlp (YouTube ingest), wget (download + healthcheck), TLS roots.
# yt-dlp ships per-arch static binaries; pick the one matching the image arch so it
# never mismatches the natively-built server binary (avoids arm64 "exec format error").
RUN apt-get update \
 && apt-get install -y --no-install-recommends ffmpeg wget ca-certificates \
 && case "${TARGETARCH}" in \
      amd64) ytdlp_asset=yt-dlp_linux ;; \
      arm64) ytdlp_asset=yt-dlp_linux_aarch64 ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2 && exit 1 ;; \
    esac \
 && wget -qO /usr/local/bin/yt-dlp \
      "https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/${ytdlp_asset}" \
 && chmod 0755 /usr/local/bin/yt-dlp \
 && rm -rf /var/lib/apt/lists/*

# Non-root user; /data owns the mounted SQLite DB.
RUN useradd --system --create-home --uid 10001 appuser \
 && mkdir -p /data \
 && chown appuser:appuser /data

WORKDIR /app
COPY --from=builder /out/server     /app/server
COPY --from=builder /out/web-public /app/web/public

# DB is a runtime volume - never baked into a layer. PORT is honored by the
# server (main.go reads it) and the HEALTHCHECK below, so `-e PORT=...` moves both.
ENV ACOUSTIC_DB_PATH=/data/acousticdna.sqlite3 \
    ACOUSTIC_TEMP_DIR=/tmp \
    PORT=8080
VOLUME ["/data"]

EXPOSE ${PORT}
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- "http://localhost:${PORT}/health" || exit 1

USER appuser
ENTRYPOINT ["/app/server"]
