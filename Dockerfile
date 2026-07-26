# syntax=docker/dockerfile:1

# --- frontend build -------------------------------------------------------
FROM node:24-alpine AS frontend
WORKDIR /src/web
RUN corepack enable
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

# --- go build (amd64 only, no cgo — modernc.org/sqlite is pure Go) --------
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/internal/web/dist ./internal/web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-s -w -X github.com/pborges/aiinventory/internal/version.Version=${VERSION}" \
    -o /out/aiinventory \
    ./cmd/aiinventory

# --- runtime ----------------------------------------------------------------
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /out/aiinventory /usr/local/bin/aiinventory

ENV PORT=8080 \
    DB_PATH=/data/aiinventory.db
VOLUME ["/data"]
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/aiinventory"]
