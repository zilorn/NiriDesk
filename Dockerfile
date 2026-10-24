# syntax=docker/dockerfile:1

FROM node:24-bookworm-slim AS frontend

WORKDIR /src/web
RUN npm install --global pnpm@11.22.0

COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm build

FROM golang:1.25-trixie AS build

WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-mod=mod

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY --from=frontend /src/web/dist ./web/dist

ARG VERSION=dev
ARG COMMIT=unknown

RUN go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
    -o /out/niridesk ./cmd/niridesk

FROM debian:trixie-slim

ARG VERSION=dev
LABEL org.opencontainers.image.title="NiriDesk" \
      org.opencontainers.image.description="Authenticated browser remote desktop for niri/wayvnc" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.licenses="MIT"

RUN apt-get update \
    && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
        ca-certificates \
        tini \
        wayvnc \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --uid 1000 --user-group --create-home --home-dir /home/niridesk niridesk

COPY --from=build /out/niridesk /usr/local/bin/niridesk

ENV HOME=/tmp \
    XDG_CONFIG_HOME=/tmp/niridesk-config \
    XDG_CACHE_HOME=/tmp/niridesk-cache \
    LISTEN_ADDR=:8080 \
    VNC_ADDR=127.0.0.1:5900 \
    WAYVNC_BIN=wayvnc \
    WAYVNCCTL_BIN=wayvncctl \
    WAYVNC_CONTROL_SOCKET=/tmp/niridesk-wayvnc.sock \
    WAYVNC_MANAGED=true \
    WAYVNC_MAX_FPS=30

USER 1000:1000
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
    CMD ["/usr/local/bin/niridesk", "healthcheck"]

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/niridesk"]
