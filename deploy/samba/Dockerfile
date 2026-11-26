# The Samba companion of UnderDoc (ADR-30, docs/samba.md): smbd and the helper underdoc-samba, which starts smbd, receives
# its audit and answers UnderDoc on a Unix socket. One companion per tenant; it sees only the directory of its tenant.
#
#   docker build -f deploy/samba/Dockerfile -t underdoc-samba .
#
# Samba comes from Debian 13 backports: 4.24 is the newest series a stable distribution carries (4.25 is not packaged
# there yet). The base image is pinned by digest and the package by version; move both together.

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY cmd/underdoc-samba ./cmd/underdoc-samba
COPY internal/samba ./internal/samba
COPY internal/version ./internal/version
ENV CGO_ENABLED=0 GOFLAGS=-trimpath GOTOOLCHAIN=local
ARG TARGETOS=linux
ARG TARGETARCH
RUN GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -ldflags "-s -w" -o /out/underdoc-samba ./cmd/underdoc-samba

FROM debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2 AS samba
ARG SAMBA_VERSION=2:4.24.7+dfsg-1~bpo13+1
# smbd, and winbindd for a companion that joins a domain (T-42); no AD DC, no printing, and no libnss-winbind: the users
# of the system are only those of the files, so a domain account maps to a UID only through a user UnderDoc applied
# (idmap_nss). The users and groups of the image are kept as a base the helper writes the Samba users after, into files
# below /var/lib/underdoc-samba that /etc/passwd and /etc/group point to.
RUN echo 'deb http://deb.debian.org/debian trixie-backports main' > /etc/apt/sources.list.d/backports.list && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends -t trixie-backports \
      "samba=${SAMBA_VERSION}" "winbind=${SAMBA_VERSION}" && \
    apt-get clean && rm -rf /var/lib/apt/lists/* /var/lib/samba/* /var/cache/samba/* /etc/samba/smb.conf && \
    ! ls /usr/lib/*/libnss_winbind.so* >/dev/null 2>&1 && ! grep -q winbind /etc/nsswitch.conf && \
    cp /etc/passwd /etc/passwd.base && cp /etc/group /etc/group.base && \
    mkdir -p /var/lib/underdoc-samba /var/lib/underdoc-samba-domain && chmod 700 /var/lib/underdoc-samba-domain && \
    cp /etc/passwd /var/lib/underdoc-samba/passwd && cp /etc/group /var/lib/underdoc-samba/group && \
    ln -sf /var/lib/underdoc-samba/passwd /etc/passwd && ln -sf /var/lib/underdoc-samba/group /etc/group && \
    ln -sf /var/lib/underdoc-samba/smb.conf /etc/samba/smb.conf
COPY --from=build /out/underdoc-samba /usr/local/bin/underdoc-samba
LABEL org.opencontainers.image.title="UnderDoc Samba companion" \
      org.opencontainers.image.source="https://github.com/Ollornog/UnderDoc" \
      org.opencontainers.image.licenses="MIT"
ENV UNDERDOC_SAMBA_RUN=/var/lib/underdoc-samba UNDERDOC_SAMBA_SOCKET=/run/underdoc-samba/helper.sock TZ=Europe/Berlin
EXPOSE 445
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --start-interval=1s --retries=3 CMD ["/usr/local/bin/underdoc-samba", "probe"]
ENTRYPOINT ["/usr/local/bin/underdoc-samba"]

# For the tests only: the same companion with smbclient, so a test can talk SMB to it from inside.
FROM samba AS test
ARG SAMBA_VERSION=2:4.24.7+dfsg-1~bpo13+1
RUN echo 'deb http://deb.debian.org/debian trixie-backports main' > /etc/apt/sources.list.d/backports.list && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends -t trixie-backports \
      "smbclient=${SAMBA_VERSION}" && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# For the tests only: a domain controller of Samba with made-up accounts (scripts/check-samba.sh, T-42). Never shipped.
FROM debian:trixie-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2 AS test-dc
ARG SAMBA_VERSION=2:4.24.7+dfsg-1~bpo13+1
RUN echo 'deb http://deb.debian.org/debian trixie-backports main' > /etc/apt/sources.list.d/backports.list && \
    apt-get update && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends -t trixie-backports \
      "samba=${SAMBA_VERSION}" "samba-ad-dc=${SAMBA_VERSION}" "samba-ad-provision=${SAMBA_VERSION}" \
      "winbind=${SAMBA_VERSION}" "smbclient=${SAMBA_VERSION}" && \
    apt-get clean && rm -rf /var/lib/apt/lists/* /etc/samba/smb.conf

FROM samba
