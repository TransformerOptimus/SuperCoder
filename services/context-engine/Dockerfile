# syntax=docker/dockerfile:1
#
# Multi-stage build for the context-engine server + worker. Both binaries are
# built from the single root Go module; the runtime image runs either one
# depending on the compose `command`.
#
# CGO is required: the tree-sitter chunkers (smacker/go-tree-sitter) are cgo
# bindings, so the builder needs gcc (present in the golang bookworm image) and
# the runtime needs glibc (debian-slim, not distroless-static).

FROM golang:1.25-bookworm AS builder
WORKDIR /src

# Dependency layer — cached unless go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Service sources (the only packages in this module; v1/ is a separate module).
COPY services/context-engine ./services/context-engine

ENV CGO_ENABLED=1
RUN go build -trimpath -o /out/server ./services/context-engine/cmd/server \
    && go build -trimpath -o /out/worker ./services/context-engine/cmd/worker

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /out/server /app/server
COPY --from=builder /out/worker /app/worker
EXPOSE 8106
# Default entrypoint is the HTTP server; the worker overrides `command`.
CMD ["/app/server"]
