# syntax=docker/dockerfile:1.29

# ─── Stage 1: build the React SPA ────────────────────────────────────────────
FROM docker.io/library/node:26-alpine AS frontend-build
WORKDIR /src
COPY ui/frontend/package.json ui/frontend/package-lock.json ./ui/frontend/
RUN cd ui/frontend && npm ci
COPY ui/frontend ./ui/frontend
RUN cd ui/frontend && npm run build && touch dist/.gitkeep

# ─── Stage 2: build the Go binary ────────────────────────────────────────────
FROM docker.io/library/golang:1.27-alpine AS go-build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-build /src/ui/frontend/dist ./ui/frontend/dist
ARG VERSION=dev
ARG COMMIT=unknown
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/nexorious \
      ./cmd/nexorious
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT}" \
      -o /out/nexctl \
      ./cmd/nexctl

# ─── Shared runtime layer (defined exactly once) ─────────────────────────────
# legendary (Epic) and nile (Amazon Games) are the storefront CLIs the sync
# adapters run. nile has no PyPI release: its tagged source is installed with
# its icon folder removed, because setuptools refuses a flat layout with two
# top-level folders ("Multiple top-level packages discovered: nile, assets").
FROM docker.io/library/alpine:3.24 AS runtime-base
RUN apk add --no-cache \
      ca-certificates \
      postgresql18-client \
      python3 py3-requests py3-filelock \
      py3-protobuf py3-pycryptodome py3-zstandard py3-json5 py3-platformdirs \
 && apk add --no-cache --virtual .pip-tmp py3-pip py3-setuptools \
 && pip install --no-cache-dir --break-system-packages --no-deps legendary-gl==0.20.34 \
 && wget -qO /tmp/nile.tar.gz https://github.com/imLinguin/nile/archive/refs/tags/v1.2.0.tar.gz \
 && echo "a06dd595248f0b4bf8fccf2b14a534391c94b810f725b6bbeaca92bc3ac2673d  /tmp/nile.tar.gz" | sha256sum -c - \
 && tar -xzf /tmp/nile.tar.gz -C /tmp \
 && rm -rf /tmp/nile-1.2.0/assets \
 && pip install --no-cache-dir --break-system-packages --no-deps --no-build-isolation /tmp/nile-1.2.0 \
 && rm -rf /tmp/nile.tar.gz /tmp/nile-1.2.0 \
 && apk del .pip-tmp \
 && addgroup -g 10001 -S nexorious \
 && adduser -u 10001 -S -G nexorious -h /app -s /sbin/nologin nexorious
WORKDIR /app
RUN mkdir -p /app/storage/backups && chown -R nexorious:nexorious /app
USER nexorious
EXPOSE 8000
ENTRYPOINT ["/app/nexorious"]
CMD ["serve"]

# ─── Target: CI release image (prebuilt binary via buildx named context) ─────
FROM runtime-base AS runtime-ci
ARG TARGETARCH
# --chmod=0755 is required: the prebuilt binary arrives via a GitHub Actions
# artifact round-trip, which does NOT preserve the executable bit (it lands as
# 0644). --chown sets owner only, not mode, so without --chmod the binary ships
# non-executable and the container fails with "exec: ... permission denied".
COPY --from=binaries --chown=nexorious:nexorious --chmod=0755 nexorious-linux-${TARGETARCH} /app/nexorious
COPY --from=binaries --chown=nexorious:nexorious --chmod=0755 nexctl-linux-${TARGETARCH} /usr/local/bin/nexctl

# ─── Target: full source build (LAST stage = default target) ─────────────────
FROM runtime-base AS runtime
# --chmod=0755 here is a no-op safety net (the go-build binary is already 0755),
# kept so both targets guarantee an executable binary unconditionally.
COPY --from=go-build --chown=nexorious:nexorious --chmod=0755 /out/nexorious /app/nexorious
COPY --from=go-build --chown=nexorious:nexorious --chmod=0755 /out/nexctl /usr/local/bin/nexctl
