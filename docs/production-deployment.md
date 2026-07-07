# Production deployment guide

Status: production-readiness hardening as of v0.10.4, targeting a
**controlled pilot** (v1.0.0-rc.1), not general availability. Do not
deploy against a production Velociraptor instance until the
prerequisites below and
[security-review-checklist-v0.10.4.md](security-review-checklist-v0.10.4.md)
are complete for your environment, then follow
[runbooks/controlled-pilot.md](runbooks/controlled-pilot.md).

## Transport model: stdio only, no ports

This server speaks MCP over stdio as a child process of an MCP client
host. It never listens on a network port, and none should be exposed
for it. If your environment wraps stdio servers in a network transport
(an MCP gateway/proxy), that wrapper is a separate component with its
own security review — nothing in this repository documents or endorses
exposing this server's stdio over a network.

Consequences worth designing around:

- Process supervision belongs to the MCP client host (it launches the
  server and keeps it alive); a standalone systemd service for the MCP
  process itself is usually wrong, because with no client attached the
  process just waits on stdin. If you need supervision, supervise the
  client host.
- "Disable access" = remove the launcher entry; there is no port to
  firewall. See [runbooks/rollback.md](runbooks/rollback.md).

## Container image

The repository root `Dockerfile` builds a static Go binary
(`CGO_ENABLED=0`) in a `golang:1.25-bookworm` stage, then copies it into
`gcr.io/distroless/static-debian12:nonroot` — no shell, no package
manager, non-root user, and no port exposed (stdio only):

```sh
docker build -t agentic-velociraptor-mcp:v0.10.4 .
```

### Pin the image tag — never run `:latest`

Build and deploy a version-pinned tag (as above), and prefer running by
digest so rollback ([runbooks/rollback.md](runbooks/rollback.md)) is a
redeploy of the previous pin:

```sh
docker images --digests agentic-velociraptor-mcp
docker run --rm -i ... agentic-velociraptor-mcp@sha256:<digest>
```

### Hardened run invocation

Run with config and secrets mounted read-only, a writable volume only
for the audit path, and the container's own attack surface reduced:

```sh
docker run --rm -i \
  --read-only \
  --cap-drop=ALL \
  --security-opt no-new-privileges \
  -v /path/to/config.yaml:/etc/agentic-velociraptor-mcp/config.yaml:ro \
  -v /path/to/secrets:/etc/agentic-velociraptor-mcp/secrets:ro \
  -v /path/to/audit:/var/log/agentic-velociraptor-mcp \
  agentic-velociraptor-mcp:v0.10.4
```

`--read-only` is safe because the image writes nothing outside the
mounted audit path (and `download_dir`, if you enable evidence
downloads — mount that writable too in that case). In Kubernetes the
equivalents are `readOnlyRootFilesystem: true`,
`allowPrivilegeEscalation: false`, `capabilities: {drop: [ALL]}`, and
`runAsNonRoot: true` (the image's user is already non-root).

### Secret mounting

The only secrets are the Velociraptor `api.config.yaml` file(s)
referenced by `read_api_config_path`/`write_api_config_path`:

- Mount them read-only from a secrets directory (or your orchestrator's
  secret store — Kubernetes `Secret` volume, Docker secret, Vault agent
  file sink). Never bake them into the image; never pass their contents
  via environment variables (the loader reads files, and file mode
  `0600` is enforced at load time).
- The config YAML itself contains no secrets (paths and policy only)
  but is still security-relevant — mount read-only and keep it change-
  controlled.
- In the read-only posture, do not mount the write identity at all;
  a secret that isn't on the host can't be misused.

### SBOM and image scanning

The build is a plain two-stage Dockerfile, so standard tooling works
unmodified — wire whichever your organization already runs into CI:

```sh
# SBOM (pick one)
docker sbom agentic-velociraptor-mcp:v0.10.4        # Docker Desktop / sbom plugin
syft agentic-velociraptor-mcp:v0.10.4               # anchore/syft

# Vulnerability scan (pick one)
trivy image agentic-velociraptor-mcp:v0.10.4
grype agentic-velociraptor-mcp:v0.10.4
```

The distroless base keeps the finding surface small; scan on every
build and before every deploy of a new pin.

## Non-container deployment

Running the bare binary is supported (see README's build-from-source
path). Apply the same discipline: dedicated non-root service account,
`0600` secrets outside version control, config change-controlled,
audit directory writable by the service account only, and the binary
built from a released tag (`git checkout v0.10.4 && go build ...`) so
rollback stays a rebuild of the previous tag.

## Prerequisites before any production connection

- Completion of [lab-validation-plan.md](lab-validation-plan.md)
  against a disposable environment matching production topology
  (v0.10.2 completed the collection/flow/hunt/IOC groups; see
  PROJECT_STATE.md's "Backend wiring status" for what remains).
- Velociraptor reader and investigator API identities provisioned per
  [velociraptor-permissions.md](velociraptor-permissions.md), reviewed
  by whoever owns Velociraptor server administration.
- `policy.allowed_artifacts` / `policy.allowed_profiles` reviewed and
  signed off by the DFIR team that will rely on this server, not left
  at defaults. Start from
  [examples/config/](../examples/config/) — `config.readonly.example.yaml`
  first, `config.controlled.example.yaml` only for the pilot stage.
- An approval mechanism (see [approval-flow.md](approval-flow.md) and
  [runbooks/approval-and-audit.md](runbooks/approval-and-audit.md))
  wired to real humans, not a stub.
- Audit log storage with retention and access control matching your
  organization's investigation-record requirements, and rotation
  configured (`audit.max_size_bytes`/`max_files`).
- [security-review-checklist-v0.10.4.md](security-review-checklist-v0.10.4.md)
  completed and archived.

## Incident response pointers

- Suspected credential compromise: revoke in Velociraptor first — this
  server has no independent revocation mechanism. Full procedure:
  [runbooks/rollback.md](runbooks/rollback.md).
- Audit sink failure blocks writes by design; the response procedure is
  in [runbooks/approval-and-audit.md](runbooks/approval-and-audit.md).

## Remaining topics for v1.0.0 (not yet covered)

- Log/audit shipping reference architecture (shipping the rotated
  files is described in the audit runbook; a specific pipeline is not
  prescribed).
- Upgrade procedure beyond tag-pin redeploys (config compatibility has
  been additive across v0.x; a formal migration policy lands with
  v1.0.0).

## Non-goals

- This server does not manage Velociraptor server installation,
  upgrades, or its own TLS/network exposure — it is a client of an
  already-operated Velociraptor deployment.
