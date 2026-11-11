# syntax=docker/dockerfile:1

FROM node:24-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --include=dev
COPY frontend/ ./
RUN npm run build

FROM golang:1.27.4-bookworm AS backend
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN go build -trimpath -ldflags="-s -w" -o /out/website ./cmd/website

FROM debian:bookworm-slim AS production
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates gosu \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system --gid 10001 website \
    && useradd --system --uid 10001 --gid website --no-create-home \
        --home-dir /nonexistent --shell /usr/sbin/nologin website \
    && mkdir -p /data \
    && chown website:website /data

WORKDIR /app
COPY --from=backend /out/website /usr/local/bin/website
COPY --from=frontend /src/frontend/dist/ ./frontend/
COPY --chmod=755 scripts/docker-entrypoint.sh /usr/local/bin/docker-entrypoint

ENV HTTP_ADDR=:8080 \
    DATABASE_PATH=/data/website.db \
    FRONTEND_DIR=/app/frontend

EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/docker-entrypoint"]
CMD ["-env", ""]
