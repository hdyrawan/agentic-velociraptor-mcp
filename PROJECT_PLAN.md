# Project plan

This is the authoritative roadmap for `agentic-velociraptor-mcp`. For
"what's actually done right now," see [PROJECT_STATE.md](PROJECT_STATE.md).

## Non-negotiable design constraints (apply to every milestone)

- Velociraptor gRPC API only, never the internal REST API.
- mTLS client authentication via Velociraptor `api.config.yaml`.
- Separate read and write API config paths; write identity used only
  after approval.
- MCP API identities must never hold `administrator`,
  `ARTIFACT_WRITER`, `SERVER_ARTIFACT_WRITER`, `EXECVE`,
  `FILESYSTEM_WRITE`, or `SERVER_ADMIN`.
- No raw VQL tool in the stable core, ever.
- No generic remote-admin-shell tool, ever.
- All caller input bound as safe parameters, never string-concatenated
  into VQL.
- Every collection, hunt start/cancel, flow cancel, and evidence
  download requires approval.
- Every tool call produces exactly one audit event with outcome
  `success`, `blocked`, or `error`; secrets never logged.
- stdio MCP transport first; HTTP/SSE/streamable HTTP only if/when
  explicitly requested.

## Stable core target: 27 tools

See [docs/tool-reference.md](docs/tool-reference.md) for the full table.
Groups: visibility (5), flow/results (6), collection (3), hunts (7),
DFIR profiles (3), DFIR workflow planning helpers (3), IOC helper (1).

## DFIR cases this design must support

See [docs/dfir-profiles.md](docs/dfir-profiles.md) for the full mapping
from investigation case to profile.

## Version roadmap

### v0.0.x — Project foundation (this run)

- Repository skeleton, Go module, basic CLI (`--help`, `--version`).
- Config structs matching the YAML model in
  [docs/configuration.md](docs/configuration.md).
- Placeholder packages: `policy`, `audit`, `approval`, `dfir`,
  `velociraptor`, `vql`, `mcpserver`.
- README, PROJECT_PLAN, PROJECT_STATE, CHANGELOG, Apache-2.0 LICENSE.
- Documentation placeholders under `docs/`.

### v0.1.0-alpha.1 — MCP skeleton (complete)

- Add the official MCP Go SDK (`github.com/modelcontextprotocol/go-sdk`)
  as a dependency.
- Start a real stdio MCP server (`internal/mcpserver.Server`, backed by
  `mcp.NewServer` / `mcp.StdioTransport`).
- Register exactly 4 read-only tools: `velo_health_check` (static mock
  response, no real Velociraptor call yet), `velo_list_dfir_profiles`,
  `velo_get_dfir_profile`, `velo_validate_dfir_profile`. The remaining
  20 planned tools are documented as `ToolSpec` metadata but
  deliberately not registered — see
  [docs/tool-reference.md](docs/tool-reference.md) and the "Tool
  Minimization" note below.
- Validated end to end against a real subprocess over stdio using the
  SDK's `CommandTransport` (tool inventory, health check, profile
  list/get/validate, and unknown-profile safe-error behavior all
  confirmed); MCP Inspector usage documented in
  [examples/inspector/README.md](examples/inspector/README.md).

### v0.1.0-alpha.2 — Real Velociraptor health check (complete)

- Load the Velociraptor read API config
  (`internal/velociraptor.LoadAPIConfig`), with file-safety checks
  (exists, regular file, owner-only permissions on POSIX) and no
  content ever logged (`APIConfig.String`/`GoString` are redacted
  unconditionally).
- Connect over gRPC with mTLS
  (`internal/velociraptor.NewGRPCClient`: `tls.X509KeyPair` +
  `x509.CertPool` + `credentials.NewTLS`, server name pinned to
  `pinned_server_name` or the upstream default
  `"VelociraptorServer"`).
- Implemented the health check via Velociraptor's own dedicated
  `API.Check` gRPC method (a minimal, hand-authored, wire-compatible
  proto mirror in `internal/velociraptor/veloapi/`, generated with
  `buf`+`protoc-gen-go`/`protoc-gen-go-grpc`, not by importing the
  upstream `Velocidex/velociraptor` module) — deliberately **not** via a
  `SELECT * FROM info()` VQL query, since `Check` is real, purpose-built,
  and requires no query construction, parameter binding, or VQL result
  parsing at all. See `internal/velociraptor/grpcclient.go`'s doc
  comment for the full rationale.
- Enforced `timeout_seconds` via `context.WithTimeout` around the RPC.
- `velo_health_check` audits every call (`success` on a healthy real or
  mock response, `error` on a real connectivity failure) and never
  reports `velociraptor_connected: true` unless the RPC actually
  succeeded — see docs/security-model.md.
- `velociraptor.read_api_config_path` is optional: empty means mock
  mode; set-but-broken fails server startup outright (fail closed)
  rather than silently falling back to mock.
- `velociraptor.write_api_config_path` is untouched by any code path in
  this milestone.

### v0.1.0 — Read-only Velociraptor visibility

- Implement: `velo_health_check`, `velo_search_clients`,
  `velo_get_client_info`, `velo_list_artifact_names`,
  `velo_get_artifact_details`, `velo_list_flows`, `velo_get_flow_status`,
  `velo_get_flow_results`.
- No collection, no hunts, no raw VQL.
- Audit every call; enforce result limits.

### v0.2.0 — Core response validation and consistent response contracts (complete, re-scoped)

Re-scoped by explicit user direction from this section's original
"Controlled single-client collection" plan (still shown further down as
the deferred goal, now unassigned to a specific version — revisit when
scheduling the next milestone). v0.2.0 as actually implemented:

- Added a shared `internal/response` envelope (`Status`: `success` /
  `empty` / `not_found` / `error`) embedded into the four visibility
  tools' response types (`velo_search_clients`, `velo_get_client_info`,
  `velo_list_artifact_names`, `velo_get_artifact_details`), replacing
  their previously ad-hoc `mode`+`message`-only shape with a
  machine-readable, documented status field. Additive to the wire
  format; no existing field was renamed or removed.
- Added a distinct `not_found` status for `velo_get_client_info` and
  `velo_get_artifact_details` (via `velociraptor.ErrClientNotFound` and
  the new `velociraptor.ErrArtifactNotFound`), previously indistinguishable
  from a generic connectivity/RPC failure.
- No write-capable Velociraptor action was added. The original
  single-client-collection scope
  (`velo_collect_artifact_with_approval`,
  `velo_collect_dfir_profile_with_approval`,
  `velo_cancel_flow_with_approval`, `velo_list_flow_uploads`,
  `velo_get_flow_upload_metadata`,
  `velo_download_flow_upload_with_approval`) remains unimplemented and
  unscheduled; see docs/tool-reference.md's "Flow and result tools" /
  "Collection tools" sections.
- Callable tool inventory unchanged: still exactly the same 8 read-only
  tools as v0.1.0.

### v0.3.0 — Read-only DFIR workflow expansion (complete)

Re-scoped by explicit user direction from the original "hunt management"
plan. v0.3.0 deliberately adds no hunt execution, collection, cancel,
download, client-side mutation, write identity use, or raw VQL. The
original hunt-management scope is deferred to a future controlled
milestone after collection/write approval foundations are implemented.

- Added three read-only workflow/planning tools, all backed only by the
  already-loaded DFIR profile registry plus local policy allowlists:
  `velo_plan_dfir_triage`, `velo_compare_dfir_profiles`, and
  `velo_find_profiles_by_artifact`.
- All new tool responses embed the v0.2.0 `internal/response.Result`
  envelope (`success` / `empty` / `not_found` / `error`) and preserve
  existing visibility/profile tool wire fields.
- Callable tool inventory increases from 8 to 11, still entirely
  read-only. The older 7 hunt-management ToolSpec entries remain
  metadata only and are not registered with MCP.

### v0.4.0 — DFIR profiles and IOC hunting

- Implement: `velo_list_dfir_profiles`, `velo_get_dfir_profile`,
  `velo_validate_dfir_profile`, `velo_hunt_ioc_with_approval`.
- Add the initial DFIR profile catalog (see
  [docs/dfir-profiles.md](docs/dfir-profiles.md)).
- Validate profile artifacts against the allowlist.
- Validate hash/IP/domain input.
- Use fixed templates and approved profile definitions only.

### v0.5.0 — Production hardening

- Docker image, non-root runtime.
- Config validation hardening.
- Audit redaction tests.
- Rate limits.
- Stable error model and response schemas.
- Integration tests; MCP Inspector validation.
- Security review checklist (see
  [docs/lab-validation-plan.md](docs/lab-validation-plan.md)).

### v1.0.0 — Stable release

- All 27 core tools implemented.
- Stable schemas.
- Full documentation.
- Lab validation report.
- Production deployment guide.
- Versioned release; changelog updated.

## Explicitly out of scope (for now, or permanently)

- Raw VQL execution tool — permanently out of scope for the stable
  core.
- Generic remote admin shell — permanently out of scope.
- HTTP/SSE/streamable HTTP transport — out of scope until explicitly
  requested after stdio is stable.
- Multi-tenant / multi-org orchestration beyond a single `org_id` —
  not currently planned; revisit if requested.

## MCP Security Best-Practice Integration

This project follows MCP security best practices in addition to Velociraptor-native security guidance.

### Transport Security

- Stdio is the default and first supported transport.
- HTTP/SSE/streamable HTTP is disabled by default.
- Future HTTP transport must require explicit enablement, authorization, safe bind defaults, and request-level authorization.
- Session IDs must never be treated as authentication or authorization.

### No Token Passthrough

The MCP server must not accept Velociraptor API tokens, certificates, bearer tokens, or `api.config.yaml` contents as tool arguments.

Velociraptor credentials are server-side secrets loaded from configured paths:

- `read_api_config_path`
- `write_api_config_path`

The MCP client never supplies downstream Velociraptor credentials per request.

### Confused Deputy Mitigation

Risky operations must bind approval to the exact request payload.

Approval-gated operations require:

- request ID
- requester
- case ID
- reason
- tool name
- target client or hunt scope
- artifact or profile name
- exact payload hash
- expiry
- one-time use

If the payload changes after approval, execution must be blocked.

### No User-Controlled URL Fetching

v1.0 tools must not accept arbitrary URLs or fetch user-supplied URLs.

Velociraptor connection information comes only from trusted server-side configuration.

### Local Server Hardening

Production deployments should run the MCP server as a dedicated low-privilege OS user or hardened container.

The process should only have access to:

- its config file
- Velociraptor API config files
- DFIR profile directory
- audit log path

It should not run as root.

### Scope and Tool Minimization

The callable tool inventory must be minimal.

Unimplemented or unsafe tools must not be registered as callable MCP tools.

Future remote/authenticated modes should use granular scopes such as:

- `velo:read`
- `velo:profiles:read`
- `velo:flows:read`
- `velo:collect`
- `velo:hunt`
- `velo:download`
- `velo:cancel`

Wildcard or omnibus scopes such as `velo:*`, `admin`, or `full-access` are not allowed.
