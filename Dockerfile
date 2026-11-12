# One static binary in an image without a shell. Both base images are pinned by digest, and Dependabot moves them.
# The build stage runs on the build platform and cross-compiles for the target (GOARCH), so an arm64 image needs no
# emulation (.github/workflows/image.yml builds amd64 and arm64).
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY docs/embed.go docs/logo.svg docs/favicon.ico ./docs/
# Reproducible: no cgo, no paths in the binary, no toolchain download.
ENV CGO_ENABLED=0 GOFLAGS=-trimpath GOTOOLCHAIN=local
ARG TARGETOS=linux
ARG TARGETARCH
RUN GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -ldflags "-s -w" -o /out/underdoc ./cmd/underdoc

FROM gcr.io/distroless/static-debian13:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
LABEL org.opencontainers.image.title="UnderDoc" \
      org.opencontainers.image.source="https://github.com/Ollornog/UnderDoc" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/underdoc /underdoc
USER 65532:65532
# The application binds to loopback by default. In a container, the published port decides who can reach it.
ENV UNDERDOC_DIR=/data UNDERDOC_ADDR=:8080 TZ=Europe/Berlin
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --start-interval=1s --retries=3 CMD ["/underdoc", "probe"]
ENTRYPOINT ["/underdoc", "serve"]
