# One static binary in an image without a shell. Both base images are pinned by digest, and Dependabot moves them.
FROM golang:1.27.3-alpine3.24@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY docs/embed.go docs/logo.svg docs/favicon.ico ./docs/
# Reproducible: no cgo, no paths in the binary, no toolchain download.
ENV CGO_ENABLED=0 GOFLAGS=-trimpath GOTOOLCHAIN=local
RUN go build -ldflags "-s -w" -o /out/underdoc ./cmd/underdoc

FROM gcr.io/distroless/static-debian13:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99
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
