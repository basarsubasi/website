# syntax=docker/dockerfile:1

FROM node:26-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --include=dev
COPY frontend/ ./
RUN npm run build

FROM golang:1.27.4-bookworm AS backend
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
COPY vendor/ ./vendor/
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/website ./cmd/website

# Writable root-filesystem directory for the non-root SQLite cache.
RUN mkdir -p /out/data && chown 10001:10001 /out/data

FROM gcr.io/distroless/static-debian13:nonroot AS production
WORKDIR /app
COPY --from=backend --chown=10001:10001 /out/data/ /data/
COPY --from=backend /out/website /usr/local/bin/website
COPY --from=frontend /src/frontend/dist/ ./frontend/

USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/website"]
CMD ["-env", ""]
