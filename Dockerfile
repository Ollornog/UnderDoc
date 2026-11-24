# One static binary in an image without a shell. The base images are pinned by digest, and Dependabot moves them.
# The build stages run on the build platform; the Go stage cross-compiles for the target (GOARCH), so an arm64 image needs
# no emulation (.github/workflows/image.yml builds amd64 and arm64).

# The JavaScript of the web editor, the PDF preview, the text of images and the diagrams, built from the sources and their lockfiles (ADR-21, scripts/build-js.sh);
# the bundles are not in the repository. --ignore-scripts: no install script of a package runs.
FROM --platform=$BUILDPLATFORM node:26.13.1-alpine3.24@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS js
WORKDIR /src
COPY web ./web
RUN for p in editor pdfjs tesseract mermaid; do \
      npm ci --prefix "web/$p" --ignore-scripts --no-audit --no-fund --loglevel=error && \
      node "web/$p/build.mjs" --out "/out/$p" || exit 1; \
    done

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY cmd ./cmd
COPY internal ./internal
COPY docs/embed.go docs/logo.svg docs/favicon.ico ./docs/
COPY --from=js /out/editor ./internal/web/assets/app/editor
COPY --from=js /out/pdfjs ./internal/web/assets/app/pdfjs
COPY --from=js /out/tesseract ./internal/web/assets/app/tesseract
COPY --from=js /out/mermaid ./internal/web/assets/app/mermaid
# Reproducible: no cgo, no paths in the binary, no toolchain download.
ENV CGO_ENABLED=0 GOFLAGS=-trimpath GOTOOLCHAIN=local
ARG TARGETOS=linux
ARG TARGETARCH
RUN GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -ldflags "-s -w" -o /out/underdoc ./cmd/underdoc

FROM gcr.io/distroless/static-debian13:nonroot@sha256:8ce6dd87e9823c5b48dcfb2c8604714ccb351ce59607327bcc4957448da279b5
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
