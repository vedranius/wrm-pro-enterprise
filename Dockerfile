# WRM Pro Enterprise - container image
#   docker build -t wrm-pro .
#   docker run -d -p 8080:8080 -v wrm-data:/data wrm-pro
# Data (database, encryption key, recordings, self-signed certificate) lives in /data.

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY remote-manager/go.mod remote-manager/go.sum ./
RUN go mod download
COPY remote-manager/ ./
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.AppVersion=${VERSION}" -o /out/wrm .

# The image is deliberately free of GPL-licensed tools. The Bitwarden CLI (GPL-3.0) is NOT
# bundled: operators who want the vault integration install it at runtime and point WRM at it
# with WRM_BITWARDEN_CLI (see docs/THIRD-PARTY.md). Node.js was removed with it.
FROM alpine:3.24
LABEL org.opencontainers.image.title="WRM Pro Enterprise" \
      org.opencontainers.image.source="https://github.com/vedranius/wrm-pro-enterprise" \
      org.opencontainers.image.licenses="Apache-2.0"
RUN apk add --no-cache ca-certificates tzdata python3 py3-pip git openssh-client rsync \
 && adduser -D -u 10001 wrm \
 && mkdir -p /data && chown wrm:wrm /data && chmod 700 /data
COPY --from=build /out/wrm /usr/local/bin/wrm
USER wrm
WORKDIR /data
ENV DB_PATH=/data/remote_manager.db \
    PORT=8080
EXPOSE 8080 3478/tcp 3478/udp
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=6s --start-period=10s --retries=3 CMD ["/usr/local/bin/wrm", "-healthcheck"]
ENTRYPOINT ["/usr/local/bin/wrm"]
