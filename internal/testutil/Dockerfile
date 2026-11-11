# syntax=docker/dockerfile:1
# Build from the repository root: docker build -f internal/testutil/Dockerfile .

FROM golang:1.27.4-bookworm AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
COPY vendor/ ./vendor/
COPY internal/ ./internal/
RUN go build -mod=vendor -trimpath -o /out/testserver ./internal/testutil/cmd/testserver

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/testserver /testserver
USER 10001:10001
EXPOSE 9000
ENTRYPOINT ["/testserver"]
CMD ["-s3-only", "-addr", ":9000"]
