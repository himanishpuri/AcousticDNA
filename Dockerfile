# syntax=docker/dockerfile:1

############################
# Stage 1 - Go builder
############################
FROM golang:1.25.5-bookworm AS builder

WORKDIR /src

# Dependency layer: cached unless go.mod/go.sum change. The module cache mount
# survives across builds so re-downloads only happen when go.sum actually moves.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# Server: CGO-free static binary (glebarez pure-Go sqlite - KEEP invariant).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/server ./cmd/server

# WASM fingerprint module (js/wasm build-tag path) + runtime glue, staged with
# the static web assets. Server and WASM are built from the same source tree,
# so createAddress (WASM) and IsValidHash (server) stay in lockstep.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" \
      -o web/public/fingerprint.wasm ./cmd/wasm \
 && cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/public/wasm_exec.js \
 && cp -r web/public /out/web-public

############################
# Stage 2 - static ffmpeg fetch (thrown away; not in final image)
############################
# The app only shells out to `ffmpeg` (mono 16-bit PCM @ 11025Hz resample).
# apt's ffmpeg drags in ~200 pkgs of X11/mesa/GL that headless audio never uses,
# so we grab a fully-static single binary instead and copy just that.
FROM debian:bookworm-slim AS ffmpeg
ARG TARGETARCH
WORKDIR /dl
RUN apt-get update \
 && apt-get install -y --no-install-recommends wget xz-utils ca-certificates \
 && case "${TARGETARCH}" in \
      amd64) ff_arch=amd64 ;; \
      arm64) ff_arch=arm64 ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2 && exit 1 ;; \
    esac \
 && asset="ffmpeg-release-${ff_arch}-static.tar.xz" \
 && base="https://johnvansickle.com/ffmpeg/releases" \
 && wget -q "${base}/${asset}" "${base}/${asset}.md5" \
 && md5sum -c "${asset}.md5" \
 && tar -xJf "${asset}" \
 && mkdir -p /out \
 && install -m0755 "$(find . -type f -name ffmpeg | head -1)" /out/ffmpeg \
 && /out/ffmpeg -version

############################
# Stage 3 - runtime
############################
FROM debian:bookworm-slim AS runtime

# Bump to match the yt-dlp version pinned by ws5.
ARG YTDLP_VERSION=2025.06.30
# Auto-populated by BuildKit from the target platform (amd64 or arm64).
ARG TARGETARCH

# Only two runtime pkgs now: wget (yt-dlp download + healthcheck) and TLS roots.
# ffmpeg arrives as a static binary from the ffmpeg stage; yt-dlp ships its own
# per-arch static binary, matched to the image arch to avoid "exec format error".
RUN apt-get update \
 && apt-get install -y --no-install-recommends wget ca-certificates \
 && case "${TARGETARCH}" in \
      amd64) ytdlp_asset=yt-dlp_linux ;; \
      arm64) ytdlp_asset=yt-dlp_linux_aarch64 ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2 && exit 1 ;; \
    esac \
 && wget -qO /usr/local/bin/yt-dlp \
      "https://github.com/yt-dlp/yt-dlp/releases/download/${YTDLP_VERSION}/${ytdlp_asset}" \
 && chmod 0755 /usr/local/bin/yt-dlp \
 && rm -rf /var/lib/apt/lists/*

COPY --from=ffmpeg /out/ffmpeg /usr/local/bin/ffmpeg

# Non-root user; /data owns the mounted SQLite DB.
RUN useradd --system --create-home --uid 10001 appuser \
 && mkdir -p /data \
 && chown appuser:appuser /data

WORKDIR /app
COPY --from=builder /out/server     /app/server
COPY --from=builder /out/web-public /app/web/public
COPY docker-entrypoint.sh /app/entrypoint.sh
RUN chmod 0755 /app/entrypoint.sh

# DB is a runtime volume - never baked into a layer. PORT is honored by the
# server (main.go reads it) and the HEALTHCHECK below, so `-e PORT=...` moves both.
ENV ACOUSTIC_DB_PATH=/data/acousticdna.sqlite3 \
    ACOUSTIC_TEMP_DIR=/tmp \
    PORT=8080
VOLUME ["/data"]

EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- "http://localhost:${PORT}/health" || exit 1

USER appuser
ENTRYPOINT ["/app/entrypoint.sh"]
