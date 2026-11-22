# syntax=docker/dockerfile:1

# One Dockerfile for both services: the SERVICE build argument selects the
# binary from cmd/. Both images share the module download layer.
#
#   docker build --build-arg SERVICE=tracker-api -t clickstream/tracker-api .
#   docker build --build-arg SERVICE=analytics-processor -t clickstream/analytics-processor .

# Build stage: Go toolchain and module cache, about 300 MB, not shipped.
# BUILDPLATFORM: the compiler runs natively even when the image is built for
# another architecture, Go cross-compiles through GOOS and GOARCH.
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS build

WORKDIR /src

# Dependencies first: this layer is rebuilt only when go.mod or go.sum
# change, not on every code edit.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG SERVICE
ARG TARGETOS
ARG TARGETARCH

# CGO_ENABLED=0: a static binary without libc, runs on any base image.
# -trimpath: no local paths in the binary and in panic stack traces.
# -ldflags "-s -w": no symbol table and DWARF, the binary is about 30% smaller.
# The build cache mount keeps compiled packages between builds.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    test -n "$SERVICE" || { echo "build argument SERVICE is required" >&2; exit 1; }; \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w" -o /out/app ./cmd/$SERVICE

# Runtime stage: only the binary on a small base image.
# Alpine, not scratch or distroless: busybox wget is needed for the
# tracker-api healthcheck in docker-compose.yml, sh helps to debug with
# docker compose exec.
FROM alpine:3.23

# The service does not need root.
RUN adduser -D -H -u 10001 app
USER app

COPY --from=build /out/app /usr/local/bin/app

# Exec form: the binary is PID 1 and receives SIGTERM from docker stop
# itself. The shell form would start sh, which does not forward signals.
ENTRYPOINT ["/usr/local/bin/app"]
