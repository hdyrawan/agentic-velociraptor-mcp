# Production deployment guide

Status: partial. A hardened container image now exists (see "Container
image" below); the remaining topics are still scheduled for v1.0.0. Do
not deploy this server against a production Velociraptor instance until
the full checklist below is complete for your environment.

## Container image

The repository root `Dockerfile` builds a static Go binary (`CGO_ENABLED=0`)
in a `golang:1.25-bookworm` stage, then copies it into
`gcr.io/distroless/static-debian12:nonroot` — no shell, no package
manager, non-root user, and no port exposed (stdio only):

```sh
docker build -t agentic-velociraptor-mcp:latest .
```

Run it with your config and (if `read_api_config_path`/
`write_api_config_path` are set) the referenced `api.config.yaml` files
mounted read-only, plus a writable volume for the audit log path
configured in `audit.path`:

```sh
docker run --rm -i \
  -v /etc/agentic-velociraptor-mcp/config.yaml:/etc/agentic-velociraptor-mcp/config.yaml:ro \
  -v /etc/agentic-velociraptor-mcp/secrets:/etc/agentic-velociraptor-mcp/secrets:ro \
  -v /var/log/agentic-velociraptor-mcp:/var/log/agentic-velociraptor-mcp \
  agentic-velociraptor-mcp:latest
```

See the root [README.md](../README.md#quick-start) for a runnable
mock-mode example with no Velociraptor server required.

## Prerequisites (once implemented)

- Completion of [lab-validation-plan.md](lab-validation-plan.md) against
  a disposable environment matching production topology.
- Velociraptor reader and investigator API identities provisioned per
  [velociraptor-permissions.md](velociraptor-permissions.md), reviewed by
  whoever owns Velociraptor server administration.
- `policy.allowed_artifacts` / `policy.allowed_profiles` reviewed and
  signed off by the DFIR team that will rely on this server, not left at
  defaults.
- An approval mechanism (see [approval-flow.md](approval-flow.md)) wired
  to real humans, not a stub.
- Audit log storage with retention and access control matching your
  organization's investigation-record requirements.

## Planned topics (v1.0.0)

- Read-only root filesystem enforcement at the orchestrator level
  (Kubernetes `readOnlyRootFilesystem: true` / equivalent), since the
  image itself writes nothing outside the mounted audit path.
- Secrets management for `read_api_config_path` /
  `write_api_config_path` (e.g. mounted from a secret store, not baked
  into the image).
- Process supervision / restart policy for the stdio MCP server process,
  and how it's wired to whatever MCP client host runs alongside it.
- Log/audit shipping to central storage.
- Upgrade procedure (config compatibility across versions, migration of
  `profiles/*.yaml`).
- Incident response: what to do if the write API identity's credentials
  are suspected compromised (revoke in Velociraptor first, this server
  has no independent revocation mechanism).

## Non-goals

- This server does not manage Velociraptor server installation,
  upgrades, or its own TLS/network exposure — it is a client of an
  already-operated Velociraptor deployment.
