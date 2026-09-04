# syntax=docker/dockerfile:1.7

FROM golang:1.24.1-bookworm AS server-build

ARG BUILD_VERSION=city311-development

WORKDIR /src/server

COPY server/go.mod server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY server/ ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 go build -trimpath \
      -ldflags="-s -w -X github.com/cortezaproject/corteza/server/pkg/version.Version=${BUILD_VERSION}" \
      -o /out/corteza-server ./cmd/corteza/main.go

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates curl \
    && mkdir -p /data/compose \
    && chown -R 10001:10001 /data \
    && rm -rf /var/lib/apt/lists/*

ENV HTTP_ADDR=0.0.0.0:80 \
    HTTP_API_BASE_URL=/api \
    HTTP_WEBAPP_ENABLED=false \
    PROVISION_PATH=/corteza/provision/* \
    STORAGE_PATH=/data

COPY --from=server-build /out/corteza-server /usr/local/bin/corteza-server
COPY --from=server-build /src/server/provision /corteza/provision

USER 10001:10001
WORKDIR /corteza

VOLUME ["/data"]
EXPOSE 80

HEALTHCHECK --interval=5s --timeout=3s --start-period=60s --retries=12 \
    CMD curl --fail --silent --show-error http://127.0.0.1:80/healthz >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/corteza-server"]
CMD ["serve-api"]
