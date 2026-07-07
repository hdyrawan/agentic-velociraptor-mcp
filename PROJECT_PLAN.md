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
- Every collection, hunt start/cancel, IOC hunt, flow cancel, and
  evidence download requires approval.
- Every tool call produces exactly one audit event with outcome
  `success`, `blocked`, or `error`; secrets never logged.
- stdio MCP transport first; HTTP/SSE/streamable HTTP only if/when
  explicitly requested.

## Stable core target: 28 tools (complete as of v0.7.0, preserved through v0.10.1)

See [docs/tool-reference.md](docs/tool-reference.md) for the full table.
Groups: visibility (5), flow/results (6), collection (3), hunts (7),
DFIR profiles (3), DFIR workflow planning helpers (3), IOC helper (1).
All 28 are now callable, and every one has a real typed gRPC backend
binding as of the GLM 5.2 hardening pass (see PROJECT_STATE.md's "Backend
wiring status"); see "What does not exist yet" in
[PROJECT_STATE.md](PROJECT_STATE.md) for what remains — chiefly live-lab
validation and the explicit-`client_ids` hunt-scope limitation, not
missing RPC wiring.

## DFIR cases this design must support

See [docs/dfir-profiles.md](docs/dfir-profiles.md) for the full mapping
from investigation case to profile.

## Version roadmap

### v0.8.0 — Real backend wiring review (complete)

- Preserved exactly 28 callable MCP tools; no raw VQL tool or parameter was added.
- Reviewed flow, collection, upload, hunt, and IOC backend paths against the available typed `veloapi` surface. No additional real RPC could be implemented safely in this repo yet because the needed typed bindings are absent.
- Added backend-capability checks before approval consumption for approval-gated operations, preserving approvals when a backend path is scaffolded or missing.
- Documented live-lab validation as pending for every scaffolded operation.

### GLM 5.2 hardening pass (complete, landed after v0.8.0, undocumented until v0.10.1)

- Added the typed `veloapi` bindings v0.8.0 above found missing, and wired real gRPC RPCs for every remaining group: flow list/status/results, collection start/cancel, flow uploads/download, and hunt list/status/results/preview/start/cancel. See PROJECT_STATE.md's "Backend wiring status".
- Discovered and documented `velociraptor.ErrHuntScopeClientIDsUnsupported`: real Velociraptor's hunt RPCs have no field for an explicit client-ID list.
- Hardened the approval store against cross-process races (OS-level `flock`), made the audit sanitizer recurse into nested structures, and added audit log rotation.

### v0.10.0 — Curated artifact catalog and DFIR profile expansion (complete)

- Added `catalog/artifacts.yaml` and `internal/dfir.LoadCatalog`/`ValidateProfileAgainstCatalog` as an authoring/test-time control (runtime collection still gated solely by `policy.allowed_artifacts`).
- Added 31 new catalog-verified DFIR profiles (46 total). Still exactly 28 MCP tools; no raw VQL; no agent-supplied artifact/profile mutation.

### v0.10.1 — Stabilization: docs/version drift, hunt-scope approval gate, CI (complete)

- Closed a real approval-consumption gap: the three hunt/IOC-hunt-start tools now refuse an explicit `client_ids` scope in real mode before consuming the approval, instead of only failing (after consuming it) inside `WriteClient.StartHunt`.
- Fixed README/PROJECT_STATE/PROJECT_PLAN/docs/CLI-help drift left over from the undocumented GLM 5.2 pass and v0.10.0 (stale "20 tools"/"scaffolded backend"/`v0.8.0` language).
- Added `.github/workflows/ci.yml`.

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
  `velo_download_flow_upload_with_approval`) remained unimplemented as of
  v0.2.0; see v0.4.0 below, which implements exactly this scope.
- Callable tool inventory unchanged: still exactly the same 8 read-only
  tools as v0.1.0.

### v0.3.0 — Read-only DFIR workflow expansion (complete)

Re-scoped by explicit user direction from the original "hunt management"
plan. v0.3.0 deliberately adds no hunt execution, collection, cancel,
download, client-side mutation, write identity use, or raw VQL. The
original hunt-management scope was deferred to v0.6.0.

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

### v0.4.0 — Controlled single-client collection pilot (complete)

Realigns with the original "Controlled single-client collection" scope
deferred at v0.2.0 (see that section above), implementing the six
previously-unscheduled tools named there. This is a **controlled pilot**,
not unrestricted Velociraptor write access: it adds exactly six new
tools, all still scoped to a single client per call, still no hunts,
still no raw VQL, still no destructive action anywhere in the codebase.

- Implement `velo_collect_artifact_with_approval`,
  `velo_collect_dfir_profile_with_approval`,
  `velo_cancel_flow_with_approval` (`tools_collection.go`), and
  `velo_list_flow_uploads`, `velo_get_flow_upload_metadata` (read-only),
  `velo_download_flow_upload_with_approval` (`tools_flows.go`).
- Every approval-gated tool requires `case_id`, `reason`, `requester`,
  the operation's target (client + artifact/profile/flow_id/upload_name),
  and an `approval_reference`. The reference must resolve, via
  `internal/approval.Store`, to a Request whose
  `approval.RequestFingerprint` exactly matches the call being made
  (operation, case_id, client_id, artifact/profile, parameters, flow_id,
  upload_name), that has been approved, is unexpired, and has not already
  been consumed — satisfying this file's Confused Deputy Mitigation
  section in full (request ID = approval reference, requester, case ID,
  reason, tool name implied by Operation, target, artifact/profile,
  fingerprint standing in for "exact payload hash", TTL-based expiry, and
  Store.Consume enforcing one-time use).
- No MCP tool can create (`Store.Create`) or decide (`Store.Decide`) an
  approval: only the `agentic-velociraptor-mcp approve` CLI subcommand
  can, run directly by a human operator, never over the MCP stdio
  transport. This is what prevents an LLM (or any other MCP client)
  driving tool calls from ever self-approving its own request — approval
  is a real control, not theater.
- The entire write pilot is disabled by default and only activates when
  **both** `policy.mode: controlled` and `approval.store_path` are
  configured (see `mcpserver.writePilotEnabled`); every write-capable
  tool call is still registered and callable (so its schema is
  discoverable) but responds with a blocked, audited refusal otherwise.
- `velo_download_flow_upload_with_approval` never returns raw evidence
  bytes inline in the MCP response: it writes them to a local,
  operator-configured directory (`velociraptor.download_dir`, itself a
  third required explicit setting for that one tool) using a filename
  derived only from already-validated client/flow IDs plus a random
  suffix — never from the caller-supplied `upload_name` — and reports
  path/size/SHA-256 instead.
- `velo_collect_artifact_with_approval` and
  `velo_collect_dfir_profile_with_approval` call
  `internal/velociraptor.Client`'s existing `CollectArtifact`/
  `CancelFlow`/`ListFlowUploads`/`GetFlowUploadMetadata`/
  `DownloadFlowUpload` methods; the hand-authored `veloapi` proto mirror
  (see v0.1.0-alpha.2's rationale) does not yet wire real gRPC bindings
  for these operations, so a real (non-mock) write client currently
  returns `velociraptor.ErrNotImplemented` for all of them — reported
  honestly as an `error`-status response, never fabricated success. Real
  wiring is a known limitation deferred to v0.6.0 below; all v0.4.0
  control-flow (policy gating, approval verification, audit, response
  envelopes) is validated against fake clients instead.
- Callable tool inventory increases from 14 (after v0.5.0 below) to 20:
  the six new tools above, all embedding the v0.2.0
  `internal/response.Result` envelope, plus the 14 read-only tools from
  v0.1.0-v0.5.0.

### v0.5.0 — Read-only flow/result backfill (complete)

Implemented independently of, and merged before, v0.4.0 above; the two
milestones are additive and were reconciled by rebasing v0.4.0 onto this
one. Re-scoped from generic production hardening to close the original
v0.1.0 read-only flow/result gap without adding write-capable behavior.

- Added callable `velo_list_flows`, `velo_get_flow_status`, and
  `velo_get_flow_results`.
- All three are read-only, audit every call, use strict client/flow ID
  validation, preserve the v0.2.0 response envelope, and route only
  through `Deps.ReadClient`.
- `velo_get_flow_results` enforces row and byte bounds and reports
  truncation honestly.
- Callable tool inventory increases from 11 to 14, still entirely
  read-only at this point (v0.4.0 above then adds the first
  write-capable tools on top).

### v0.6.0 — Hunt management schema/safety scaffold (complete)

Backfill of the original v0.4.0/v0.5.0 hunt-management scope from the
PROJECT_PLAN.md roadmap. All seven MCP tool handlers are implemented
and registered, but **real gRPC hunt execution is NOT yet implemented**
— the handlers invoke `Deps.WriteClient` and `Deps.Approvals` which are
currently `placeholderClient` (returns `ErrNotImplemented`) and `nil`
respectively. Real-mode write operations report "not yet available."
This milestone is a schema and safety scaffold with fake-backed tests;
real Velociraptor hunt RPC execution must be added in a follow-up.

- Implemented seven hunt management tool handlers:
  `velo_preview_hunt_scope` (RO, blocks `target_all` by default),
  `velo_start_hunt_with_approval` (approval-gated, allowlisted artifacts,
  enforces `max_hunt_clients`),
  `velo_start_dfir_hunt_with_approval` (approval-gated, profile
  allowlist),
  `velo_list_hunts` (RO), `velo_get_hunt_status` (RO),
  `velo_get_hunt_results` (RO, bounded by max_rows/max_result_bytes,
  paginated), and `velo_cancel_hunt_with_approval` (approval-gated).
- Callable tool inventory increases from 20 to 27.
- All approval-gated tool handlers enforce: fingerprint-matched approval
  verification (`verifyApproval/consumeApproval`, added in the v0.7.0 fix
  below — the schema/safety scaffold as originally merged used a weaker,
  non-fingerprint-checking `checkHuntApproval` helper; see
  [CHANGELOG.md](CHANGELOG.md)'s v0.7.0 entry), `policy` mode checks
  (read-only mode blocks writes), scope validation
  (`validation.ValidateHuntScope`), artifact/profile allowlists,
  `max_hunt_clients` caps, and `target_all` restrictions.
- All read-only hunt tools (preview, list, status, results) follow the
  existing mock/real branching with evidence-honest responses.
- Tests in `tools_hunts_test.go` use fakes/a real `approval.FileStore`
  for all dependencies; no live Velociraptor server is required.

### v0.7.0 — IOC hunting helper (complete)

Adds the last tool from the original 28-tool stable-core target
(`velo_hunt_ioc_with_approval`), and reviews v0.6.0's scaffolded backend
paths for anything safe/clear to implement for real.

- Added `velo_hunt_ioc_with_approval`: validates a `hash`, `ip`,
  `domain`, `process`, or `path` indicator, resolves it through a fixed
  `internal/vql` template to an allowlisted artifact + bound parameter,
  and starts a hunt via the same `velociraptor.HuntWriter.StartHunt`
  path `velo_start_hunt_with_approval` uses. New
  `approval.OperationHuntIOC` category; approval-gated like every other
  write tool via `verifyApproval/consumeApproval`.
- Completed `internal/vql.Bind`'s template → (artifact, parameter key)
  mapping for all 5 IOC templates — real, deterministic Go logic
  involving no gRPC call. The artifact names themselves remain
  illustrative/unverified against a live Velociraptor catalog (same
  caveat as the pre-existing `ioc_hash_hunt`/`ioc_ip_hunt`/
  `ioc_domain_hunt` DFIR profiles); real hunt-start gRPC execution
  remains scaffolded, unchanged from v0.6.0.
- Added `internal/validation.Process`/`Path` IOC validators and a
  `ValidateIOC(kind, value)` router.
- Fixed two issues found reviewing the v0.6.0 branch before landing it:
  a confused-deputy gap where hunt approvals weren't fingerprint-checked
  (any valid unconsumed approval could start/cancel a different hunt
  than approved), and three hunt-write tools registered with no MCP
  annotations. See [CHANGELOG.md](CHANGELOG.md) for details.
- Callable tool inventory increases from 27 to 28 (the full stable-core
  target).

### Remaining production-hardening items (not yet started)

This was originally planned as "v0.8.0 — Production hardening"; the
version number was used instead for the backend-wiring-review milestone
above, and the real gRPC wiring item below was completed by the GLM 5.2
pass and v0.10.1's approval-gate fix. What remains:

- Docker image, non-root runtime. — **done** (see production-deployment.md).
- Config validation hardening. — **done** (config.Validate, tested).
- Audit redaction tests. — **done**, and extended to recursive
  redaction by the GLM 5.2 pass.
- Rate limits. — not yet started.
- Stable error model and response schemas. — **done**
  (`internal/response.Result`).
- Integration tests; MCP Inspector validation. — partial: unit/handler
  tests exist; live-lab validation against a real Velociraptor server
  remains pending (docs/lab-validation-plan.md).
- Security review checklist (see
  [docs/lab-validation-plan.md](docs/lab-validation-plan.md)). — checklist
  exists; most live-server items remain unchecked.
- Real gRPC wiring for collection/cancel/upload/hunt/IOC RPCs — **done**
  (GLM 5.2 pass); **live-lab validation against a real Velociraptor lab
  remains pending.**

### v1.0.0 — Stable release (not yet started)

- All 28 core tools implemented.
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
