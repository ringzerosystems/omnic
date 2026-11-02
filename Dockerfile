# Runtime base: pure-Go static binaries, no libc, no shell. Runs as uid 65532.
# Pinned by digest; bump via Renovate/Dependabot. Declared before the first FROM so both runtime stages can use it.
ARG RUNTIME=gcr.io/distroless/static-debian13:nonroot@sha256:1c2c046bc09ed40fad370b599a0b1ae7987f55b01e247cf27a7c27cd97e5bbc7

# Builder: Debian/glibc-based so it matches the distroless runtime. Never use golang:alpine here.
FROM golang:1.27-trixie AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/omnic ./cmd/omnic
COPY internal ./internal
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-X main.version=${VERSION}" -o /out/omnic ./cmd/omnic

# Debug builder: optimizations off so breakpoints and variables resolve, plus the go.mod-pinned dlv.
FROM build AS build-debug
ARG VERSION=dev-debug
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -gcflags='all=-N -l' -ldflags="-X main.version=${VERSION}" -o /out/omnic ./cmd/omnic && \
    CGO_ENABLED=0 go build -o /out/dlv github.com/go-delve/delve/cmd/dlv

# Debug image: dlv headless on :2345 wrapping the server; compose `command` args land after `--`.
# Local only (docker-compose.debug.yml); never a deploy artifact.
FROM ${RUNTIME} AS debug
COPY --from=build-debug /out/omnic /omnic
COPY --from=build-debug /out/dlv /dlv
EXPOSE 8080 6060 2345
ENTRYPOINT ["/dlv", "exec", "/omnic", "--headless", "--listen=:2345", "--api-version=2", "--accept-multiclient", "--continue", "--"]

# Production image. Last stage, so it is also the default build target.
FROM ${RUNTIME} AS runtime
COPY --from=build /out/omnic /omnic
EXPOSE 8080 6060
ENTRYPOINT ["/omnic"]
