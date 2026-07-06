# syntax=docker/dockerfile:1

# Builder: compiles a static agentic-velociraptor-mcp binary. Pure Go
# dependencies only (grpc-go, protobuf, yaml.v3) so CGO_ENABLED=0 works
# with no extra build tooling.
FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
    -o /out/agentic-velociraptor-mcp \
    ./cmd/agentic-velociraptor-mcp

# Runtime: distroless nonroot, no shell, no package manager — matches
# docs/security-model.md's "Local Server Hardening" guidance (dedicated
# low-privilege user, minimal attack surface, no root).
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/agentic-velociraptor-mcp /app/agentic-velociraptor-mcp
COPY --from=builder --chown=nonroot:nonroot /src/profiles /app/profiles

USER nonroot

# stdio transport only — no port is exposed or listened on.
ENTRYPOINT ["/app/agentic-velociraptor-mcp"]
CMD ["--config", "/etc/agentic-velociraptor-mcp/config.yaml", "--profiles-dir", "/app/profiles"]
