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

# Keep the existing service UID so previously created SQLite files stay writable.
RUN mkdir -p /out/data && chown 10001:10001 /out/data

# Local integration-test fixture; never copied into the production image.
FROM backend AS test-fixture-build
RUN go build -trimpath -o /out/testserver ./internal/testutil/cmd/testserver

FROM gcr.io/distroless/static-debian13:nonroot AS test-fixture
COPY --from=test-fixture-build /out/testserver /testserver
USER 10001:10001
ENTRYPOINT ["/testserver"]
CMD ["-s3-only", "-addr", ":9000"]

FROM gcr.io/distroless/static-debian13:nonroot AS production
WORKDIR /app
COPY --from=backend --chown=10001:10001 /out/data/ /data/
COPY --from=backend /out/website /usr/local/bin/website
COPY --from=frontend /src/frontend/dist/ ./frontend/

ENV HTTP_ADDR=:8080 \
    DATABASE_PATH=/data/website.db \
    FRONTEND_DIR=/app/frontend

USER 10001:10001
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/website"]
CMD ["-env", ""]
