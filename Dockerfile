# syntax=docker/dockerfile:1.24

# ─── Stage 1: build the React SPA ────────────────────────────────────────────
FROM docker.io/library/node:24-alpine AS frontend-build
WORKDIR /src
COPY ui/frontend/package.json ui/frontend/package-lock.json ./ui/frontend/
RUN cd ui/frontend && npm ci
COPY ui/frontend ./ui/frontend
RUN cd ui/frontend && npm run build && touch dist/.gitkeep

# ─── Stage 2: build the Go binary ────────────────────────────────────────────
FROM docker.io/library/golang:1.26-alpine AS go-build
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

# ─── Stage 3: minimal runtime ────────────────────────────────────────────────
FROM docker.io/library/alpine:3.23 AS runtime
RUN apk add --no-cache \
      ca-certificates \
      postgresql18-client \
      python3 py3-requests py3-filelock \
 && apk add --no-cache --virtual .pip-tmp py3-pip \
 && pip install --no-cache-dir --break-system-packages --no-deps legendary-gl==0.20.34 \
 && apk del .pip-tmp \
 && addgroup -g 10001 -S nexorious \
 && adduser -u 10001 -S -G nexorious -h /app -s /sbin/nologin nexorious

WORKDIR /app
COPY --from=go-build /out/nexorious /app/nexorious
RUN mkdir -p /app/storage /app/storage/backups && chown -R nexorious:nexorious /app

USER nexorious
EXPOSE 8000
ENTRYPOINT ["/app/nexorious"]
CMD ["serve"]
