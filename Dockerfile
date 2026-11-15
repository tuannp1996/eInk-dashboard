# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/usage-agent ./cmd/usage-agent

# ---- runtime: Debian + Chromium for HTML → PNG rendering ----
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      chromium ca-certificates tzdata curl \
      fonts-inter fonts-noto-core fonts-dejavu-core \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --system --uid 10001 --home /app --shell /usr/sbin/nologin viaiz
WORKDIR /app
COPY --from=build /out/server /out/usage-agent /app/
COPY config /app/config
COPY mock /app/mock
RUN mkdir -p /app/data && chown -R viaiz /app/data
USER viaiz
ENV CHROME_PATH=/usr/bin/chromium \
    CONFIG_FILE=/app/config/config.yaml \
    CACHE_FILE=/app/data/cache.json \
    HOME=/app
EXPOSE 8080
HEALTHCHECK --interval=60s --timeout=5s --start-period=20s \
  CMD curl -fsS "http://127.0.0.1:8080${BASE_PATH:-}/healthz" >/dev/null || exit 1
ENTRYPOINT ["/app/server"]
