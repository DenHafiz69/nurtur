# syntax=docker/dockerfile:1

FROM golang:1.24-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/nurtur ./cmd/nurtur

# glibc base: tursogo loads a prebuilt native library through purego.
# CGO stays disabled, but the runtime needs glibc (Debian slim, not Alpine).
FROM debian:bookworm-slim AS runtime

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home-dir /data --shell /usr/sbin/nologin nurtur \
    && mkdir -p /data \
    && chown -R nurtur:nurtur /data

COPY --from=build /out/nurtur /usr/local/bin/nurtur

USER nurtur
WORKDIR /data

# The Turso Go library extracts its embedded native library into a cache dir.
# /tmp is writable for the non-root user and is re-created if the cache clears.
ENV NURTUR_ADDR=:8080 \
    NURTUR_DB=/data/nurtur.db \
    NURTUR_NIGHT_START=19:00 \
    NURTUR_NIGHT_END=07:00 \
    TURSO_GO_CACHE_DIR=/tmp/turso-go

EXPOSE 8080
VOLUME ["/data"]

HEALTHCHECK --interval=30s --timeout=4s --start-period=15s --retries=3 \
    CMD curl -fsS http://127.0.0.1:8080/healthz >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/nurtur"]
