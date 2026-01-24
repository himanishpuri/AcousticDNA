FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git make gcc musl-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o acousticdna-server ./cmd/server
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o acousticdna-cli ./cmd/cli

FROM alpine:latest

RUN apk add --no-cache \
    ffmpeg \
    ca-certificates \
    python3 \
    py3-pip \
    gcc \
    && pip3 install --no-cache-dir yt-dlp --break-system-packages

WORKDIR /app

COPY --from=builder /app/acousticdna-server .
COPY --from=builder /app/acousticdna-cli .
COPY web ./web

RUN mkdir -p /app/data /app/temp

ENV ACOUSTIC_DB_PATH=/app/data/acousticdna.sqlite3
ENV ACOUSTIC_TEMP_DIR=/app/temp
ENV PORT=8080

EXPOSE 8080

VOLUME ["/app/data", "/app/temp"]

CMD ["./acousticdna-server", "-port", "8080", "-db", "/app/data/acousticdna.sqlite3", "-temp", "/app/temp"]
