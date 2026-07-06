# Project state

Last updated: 2026-07-06 (v0.4.0, controlled single-client collection
pilot, rebased onto v0.5.0's read-only flow/result backfill).

## Current milestone

**v0.4.0 — Controlled single-client collection pilot.** Complete. This
is this project's **first write-capable Velociraptor feature**, and it
is deliberately a controlled pilot, not unrestricted write access: no
hunts, no multi-client collection, no raw VQL, no destructive action.
Implemented independently of, and merged after, v0.5.0's read-only
flow/result backfill (see "Previous milestone" below); this branch was
rebased onto that work rather than the two conflicting.

- Added six new tools: `velo_collect_artifact_with_approval`,
  `velo_collect_dfir_profile_with_approval`,
  `velo_cancel_flow_with_approval` (`internal/mcpserver/tools_collection.go`),
  `velo_list_flow_uploads`, `velo_get_flow_upload_metadata` (read-only),
  and `velo_download_flow_upload_with_approval`
  (`internal/mcpserver/tools_flows.go`).
- Every approval-gated tool requires `case_id`, `reason`, `requester`,
  its target (client plus artifact/profile/flow_id/upload_name as
  applicable), and an `approval_reference`. The reference must resolve
  to an `internal/approval.Store` record that is approved, unconsumed,
  unexpired, and whose `approval.RequestFingerprint` exactly matches the
  operation/case_id/client_id/artifact/profile/parameters/flow_id/
  upload_name of the actual call — a fingerprint mismatch (e.g. approved
  for one artifact, called for another) is blocked, not silently
  substituted.
- **No MCP tool can create or decide an approval.** `approval.Store`'s
  `Create`/`Decide` methods are only ever called from the new
  `agentic-velociraptor-mcp approve` CLI subcommand
  (`cmd/agentic-velociraptor-mcp/main.go`), run directly by a human
  operator against the same on-disk `approval.FileStore` file the MCP
  server reads — never over the MCP stdio transport. This is the
  concrete mechanism that prevents an LLM (or any other MCP client)
  driving tool calls from self-approving its own request.
- The whole write pilot is off by default: `mcpserver.writePilotEnabled`
  requires **both** `policy.mode: controlled` and `approval.store_path`
  configured, or every approval-gated tool call is refused (audited
  `blocked`) before touching Velociraptor at all.
  `velo_download_flow_upload_with_approval` has a third gate,
  `velociraptor.download_dir`, since it is the one tool that discloses
  raw evidence — and even then it never returns the bytes inline in the
  MCP response, only a local path/size/SHA-256 after writing them to
  that directory under a filename derived only from already-validated
  client/flow IDs (never the caller-supplied `upload_name`).
- `internal/approval.FileStore` is the first real `approval.Store`
  implementation (previously interface-only). JSON-file-backed,
  re-reads on every call (no in-memory caching) specifically so an
  `approve` CLI invocation from a separate process becomes visible to a
  running MCP server without a restart. `Consume` is called before the
  underlying Velociraptor call, so a single human approval authorizes at
  most one execution attempt regardless of whether that attempt
  succeeds.
- **Known limitation**: the hand-authored `veloapi` proto mirror (see
  v0.1.0-alpha.2's rationale for why this project doesn't import
  upstream `Velocidex/velociraptor`) does not yet wire real gRPC bindings
  for `CollectArtifact`/`CancelFlow`/upload RPCs. A real (non-mock) write
  client therefore currently returns `velociraptor.ErrNotImplemented` for
  every write operation, reported honestly as an `error`-status response
  — never fabricated success. All v0.4.0 control-flow (policy gating,
  approval verification/consumption, audit, response envelopes) is
  validated against fake `velociraptor.Client` implementations instead;
  see `internal/mcpserver/tools_collection_test.go` and
  `tools_flows_test.go`. Real RPC wiring is deferred to v0.6.0.
- Callable tool inventory increases from 14 (v0.1.0-v0.5.0, all
  read-only) to 20: the six tools above are the only write-capable ones
  anywhere in this codebase.

## Previous milestone

**v0.5.0 — Read-only flow/result backfill.** Complete at the MCP handler
and read-client interface layer. This milestone closes the original
v0.1.0 tool-surface gap without adding collection execution, hunt start
or cancel, flow cancel, downloads, write identity use, client/server
mutation, or raw VQL exposure.

- Added three read-only flow/result tools: `velo_list_flows`,
  `velo_get_flow_status`, and `velo_get_flow_results`.
- All three validate `client_id` / `flow_id` before backend calls, embed
  `internal/response.Result`, honestly report mock mode, and audit every
  invocation.
- `velo_get_flow_results` applies `velociraptor.max_rows` and
  `velociraptor.max_result_bytes`, reports `truncated` / `next_cursor`,
  and records row/byte counts in audit events.
- Callable tool inventory was 14, all read-only, before v0.4.0 above
  added the first six write-capable tools.
- Real Velociraptor flow RPC plumbing is still not implemented in
  `grpcClient`; real-mode calls through the current backend therefore
  produce a structured `error` rather than fake data. Handler behavior is
  covered with fake read-client tests.

## Live lab validation (2026-07-06)

Validated the 8-tool read-only surface against a real Velociraptor lab
(container `velociraptor-lab`, image `wlambert/velociraptor:latest`,
**Velociraptor version 0.76.3**), using the lab's pre-generated
**`mcp-reader` reader API identity** (role `reader`, not
`administrator`; the investigator identity was never used). Driven over
a real stdio subprocess (`mcp.CommandTransport` against the actual built
binary), not just unit tests against fakes.

- **Callable inventory at the time: exactly 8** — `ListTools` against the live
  subprocess returned the same 8 tools as
  `internal/mcpserver/server_test.go`'s exact-inventory test. No new
  tool had been added at that time; v0.5.0 later added the three read-only flow/result handlers.
- **No raw VQL / generic `Query` path**: reconfirmed — `grpcClient` has
  no `Query` method; a full 399-artifact real catalog dump was scanned
  for VQL-shaped text and only human-authored `description` prose
  mentioning VQL as a usage example was found, never an artifact's
  actual query body (which the generated types have no field for).
- **Client inventory: empty.** The lab has zero enrolled endpoint
  clients. `velo_search_clients` was confirmed to return an honest
  `clients: [], mode: "real"` rather than mock data, but per-client
  field-value correctness (hostname/OS/last-seen/labels/MAC addresses)
  and `velo_search_clients`'s query-matching behavior remain unverified
  against real client data — see docs/lab-validation-plan.md.
- **Tools live-validated**: `velo_health_check` (real success),
  `velo_search_clients` (empty-result path, bounded `limit`),
  `velo_get_client_info` (validation rejection, and the not-found path —
  see bug below), `velo_list_artifact_names` (allowlist-scoped vs.
  full-catalog via `policy.allow_list_all_artifacts`, and `max_rows`
  truncation against the real 399-artifact catalog), `velo_get_artifact_details`
  (real data, allowlist blocking, no VQL leakage). The three DFIR
  profile tools were confirmed present in `ListTools` but not
  separately re-exercised (they're local-file-based and lab-independent).
- **Confirmed and fixed a real bug**: `velo_get_client_info` against a
  well-formed but nonexistent client ID previously returned a
  "successful" result carrying an empty-but-present client object
  (`client_id: ""`), because Velociraptor's real `GetClient` RPC returns
  a zero-value `ApiClient` rather than an error for an unknown ID. This
  silently violated the evidence-honesty principle (looked like a real,
  if sparse, client record). Fixed: `internal/velociraptor/grpcclient.go`'s
  `GetClientInfo` now detects an empty `resp.GetClientId()` and returns
  the new `ErrClientNotFound` sentinel (`internal/velociraptor/clients.go`)
  instead, so the tool now reports `mode: "real"`, no `client` field, and
  an honest `"...client not found"` message. Covered by a new
  `TestGRPCClientGetClientInfoNotFound` unit test
  (`internal/velociraptor/grpcclient_test.go`).
- **Audit log**: every live call produced exactly one JSONL audit event
  with the correct `success`/`blocked`/`error` outcome, and no secret
  material (checked directly against the scratch `audit.jsonl` produced
  during this pass).
- **Network topology note, not a code issue**: the lab's generated
  `mcp-reader.api.config.yaml` has `api_connection_string: 0.0.0.0:8001`,
  but the lab's Docker Compose only publishes ports 8000/8889 to the
  host (port 8001, the gRPC API, is reachable only via the container's
  bridge-network IP). Validation used a local scratch copy of the reader
  config with only `api_connection_string` rewritten to that IP; the
  original secret-bearing file was never modified, and no change was
  needed in this project's own config loading/validation code.
- See docs/lab-validation-plan.md's Phase 1/Phase 2 sections for the
  full checklist with per-item results, and "Known assumptions to
  revisit" below for what's still unverified.

**Requires Go 1.25+ to build.**

## What exists

- **New in v0.4.0**: `internal/approval/filestore.go` (`FileStore`, the
  first real `Store` implementation), a `Requester`/`Parameters`/
  `FlowID`/`UploadName`-extended `approval.Request`, and an
  `approval.Status` type covering Consumed/Expired lifecycle state.
  `internal/mcpserver/tools_collection.go` and `tools_flows.go` now
  implement six tools (see "Current milestone"). `internal/config`
  gained `ApprovalConfig` (`store_path`/`ttl_seconds`) and
  `VelociraptorConfig.DownloadDir`. `cmd/agentic-velociraptor-mcp`
  gained the `approve` subcommand and wires a real `WriteClient`/
  `VelociraptorWriteMode` from `write_api_config_path`, mirroring the
  existing read-client wiring.
- **New in v0.3.0**: `internal/mcpserver/tools_workflow.go`, registering
  three read-only local workflow helpers: `velo_plan_dfir_triage`,
  `velo_compare_dfir_profiles`, and `velo_find_profiles_by_artifact`.
  These tools return profile recommendations, profile artifact overlap,
  and artifact-to-profile coverage using only the loaded profile registry
  and policy allowlists. They do not call Velociraptor or use the write
  client.
- **New in v0.2.0**: `internal/response` (the `Result`/`Status` envelope
  described under "Current milestone" above) and
  `velociraptor.ErrArtifactNotFound`. Everything else below predates
  v0.2.0; only `internal/mcpserver/tools_visibility.go`'s four output
  types and `internal/velociraptor/grpcclient.go`'s
  `GetArtifactDetails` changed to use them (see "Current milestone").
- Go module `github.com/hdyrawan/agentic-velociraptor-mcp` (go 1.25).
- Dependencies: `gopkg.in/yaml.v3`, `github.com/modelcontextprotocol/go-sdk`
  v1.6.1 (MCP protocol), `google.golang.org/grpc` v1.82.0 and
  `google.golang.org/protobuf` v1.36.11 (new this milestone, for the
  real Velociraptor gRPC client), plus their transitive deps. No other
  direct dependencies.
- `cmd/agentic-velociraptor-mcp`: CLI with `--version`, `--help`,
  `--config` (required to run), `--profiles-dir` (default `profiles`,
  now falls back to a path relative to the executable if the
  cwd-relative lookup fails — see `resolveProfilesDir`). With a valid
  `--config`, the default command loads config, builds all
  dependencies (constructing a real Velociraptor gRPC client if
  `velociraptor.read_api_config_path` is set, or the mock placeholder
  if it's empty), and runs a real MCP server over stdio. A missing
  config file, or a *configured-but-broken* `read_api_config_path`,
  both fail closed (exit 1) without ever starting the transport.
- `internal/mcpserver`: 20 registered tools (up from 11 as of v0.3.0; 14
  after v0.5.0's read-only flow/result backfill; 20 after this
  milestone's six write-capable tools):
  `velo_health_check`, `velo_search_clients`, `velo_get_client_info`,
  `velo_list_artifact_names`, `velo_get_artifact_details`,
  `velo_list_dfir_profiles`, `velo_get_dfir_profile`,
  `velo_validate_dfir_profile`, `velo_plan_dfir_triage`,
  `velo_compare_dfir_profiles`, `velo_find_profiles_by_artifact`,
  `velo_list_flows`, `velo_get_flow_status`, `velo_get_flow_results`,
  `velo_collect_artifact_with_approval`,
  `velo_collect_dfir_profile_with_approval`,
  `velo_cancel_flow_with_approval`, `velo_list_flow_uploads`,
  `velo_get_flow_upload_metadata`,
  `velo_download_flow_upload_with_approval`. All
  five visibility tools share
  `velo_health_check`'s existing mock/real branching and
  evidence-honesty pattern:
  - `"mock"` (default, `read_api_config_path` empty): no Velociraptor
    call; response carries `mode: "mock"` and an explanatory message.
  - `"real"`: calls the corresponding `Deps.ReadClient` method
    (`SearchClients`/`GetClientInfo`/`ListArtifactNames`/
    `GetArtifactDetails`). A connectivity or lookup failure is reported
    as a normal structured result (empty/`nil` data plus a `message`),
    not a Go-level tool error.
  - Input validation (`validation.ClientID`, `validation.ArtifactName`,
    `validation.SearchQuery`) and, for the two artifact tools, an
    allowlist check (`policy.AllowListAllArtifacts()` /
    `policy.ArtifactAllowed`) happen before any mode branching, and
    *do* produce a Go-level error with a `blocked` audit outcome — these
    are static request defects, not Velociraptor connectivity data.
  Only hunt/IOC execution tools remain unregistered `ToolSpec` metadata;
  the exact-tool-inventory test now expects 20
  (`TestNewRegistersExactlyTwentyTools`), and
  `TestNewNeverRegistersUnsafeTools` guards against a
  hunt/raw-VQL-named tool ever becoming callable while allowlisting the
  six known write-capable tool names introduced this milestone.
- `internal/velociraptor`: gained four more real methods this milestone,
  on top of the existing `HealthCheck`.
  - `veloapi/`: split into `health.proto` (health messages only),
    `visibility.proto` (new: `SearchClientsRequest`/`Response`,
    `ApiClient`, `AgentInformation`, `Uname`, `GetClientRequest`,
    `GetArtifactsRequest`, `ArtifactDescriptors`, `Artifact`,
    `ArtifactParameter`), and `api.proto` (the `service API`
    definition, now declaring `Check`, `ListClients`, `GetClient`,
    `GetArtifacts`). All still hand-authored/buf-generated, still no
    dependency on the upstream `Velocidex/velociraptor` module. Every
    message deliberately omits fields this project doesn't need —
    notably `Artifact` has no field for `sources` (upstream's VQL query
    body), so a real server's response can never surface VQL text
    through these generated types even by accident.
  - `grpcclient.go`: `grpcClient` gained `SearchClients`,
    `GetClientInfo`, `ListArtifactNames`, `GetArtifactDetails`, each
    timeout-bounded and routed through the existing `sanitizeTLSError`.
    `NewGRPCClient` gained a `maxRows` parameter
    (`config.VelociraptorConfig.MaxRows`), enforced via
    `boundLimit`/`effectiveMaxRows` (falls back to an internal
    `defaultMaxRows = 100` if non-positive) on both the outgoing
    request and the returned result count — a server that ignores the
    requested limit is still truncated client-side.
  - `client.go`'s placeholder/`NewClient()` is unchanged and still used
    for `WriteClient` unconditionally, and for `ReadClient` when mock
    mode is selected.
- `internal/validation`: added `SearchQuery` (length cap, rejects
  control characters) for `velo_search_clients`'s free-text filter.
- `internal/policy`: added `Engine.AllowListAllArtifacts()`.
- `internal/config`: unchanged this milestone (no new config fields —
  `max_rows` already existed and is now actually consumed by
  `NewGRPCClient`).
- Tests: `internal/mcpserver/tools_workflow_test.go` covers v0.3.0
  success, empty, not_found, and validation-error paths for the three
  workflow tools; `server_test.go` now verifies exactly 14 callable
  read-only tools and exercises the new tools over an MCP session. Older
  tests: `internal/velociraptor/grpcclient_test.go` gained
  `fakeClientSearcher`/`fakeClientGetter`/`fakeArtifactCatalog` and
  success/error/timeout/limit-bounding/no-secret-leakage tests for all
  four new methods. `internal/mcpserver/tools_visibility_test.go`
  gained a `fakeVisibilityClient` and mock-mode/real-mode
  success/error/invalid-input/allowlist-gating tests for all four new
  handlers. `server_test.go` updated to the 8-tool inventory and gained
  the never-registers-unsafe-tools test plus an MCP-session-level call
  test for the four new tools. `internal/validation` gained
  `TestSearchQuery`. All tests use fakes/fixtures — none require a live
  Velociraptor server or real certificates.
- **Not yet verified against a real Velociraptor server** — no lab
  server was available in this environment; see
  docs/lab-validation-plan.md's unchecked Phase 2 items (result field
  correctness, `allow_list_all_artifacts` behavior, `max_rows`
  enforcement, and confirming no VQL text leaks through in practice, all
  against a real server).
- Docs updated for this milestone: README, PROJECT_STATE,
  docs/tool-reference.md, docs/security-model.md (extended "Dependency
  surface" and "Evidence honesty" sections), docs/lab-validation-plan.md
  (Phase 2 filled in), docs/configuration.md (`read_api_config_path`/
  `max_rows` field notes updated).

## What does not exist yet

- Hunt management and the IOC hunting tool — deferred indefinitely;
  no hunt of any kind exists in this codebase, by design (see
  PROJECT_PLAN.md's non-negotiable constraints).
- Real gRPC wiring for `CollectArtifact`/`CancelFlow`/upload RPCs — the
  six v0.4.0 tools exist and are fully policy/approval/audit-gated, but
  `grpcClient` still delegates these to the embedded placeholder
  (`ErrNotImplemented`) for a real (non-mock) write client; see v0.4.0's
  "known limitation" above and PROJECT_PLAN.md's v0.6.0 entry. All
  current test coverage for these tools uses fake `velociraptor.Client`
  implementations.
- Real gRPC wiring for `ListFlows`/`GetFlowStatus`/`GetFlowResults` on
  the read side (v0.5.0's tools exist and are fully validated/audited,
  but `grpcClient` still delegates these to the embedded placeholder).
- Any RPC beyond `Check`/`ListClients`/`GetClient`/`GetArtifacts` — no
  `Query`.
- Any audit sanitizer implementation beyond a flat top-level map
  redactor.
- HTTP/SSE/streamable HTTP transport — intentionally out of scope.
- Real-server validation of `velo_search_clients`/`velo_get_client_info`
  against actual enrolled endpoint clients — the 2026-07-06 lab pass (see
  above) had zero enrolled clients, so field-value correctness
  (hostname/OS/last-seen/labels/MAC addresses) and
  `SearchClientsRequest.query`'s real matching grammar remain unverified.
  The next person with access to a lab with at least one enrolled client
  should run through docs/lab-validation-plan.md's remaining unchecked
  Phase 2 items. The v0.4.0 write-capable tools have never been
  exercised against a live Velociraptor server at all (see the known
  limitation above).
- Docker image, rate limiting, further integration tests (v0.6.0
  concerns).

## Known assumptions to revisit

- Go module path assumed as `github.com/hdyrawan/agentic-velociraptor-mcp`
  (not confirmed against an actual GitHub remote at time of writing).
- `internal/validation.ClientID`'s regex (`C.` + 16 hex chars) is
  syntactically well-formed enough to reach a real `GetClient` call (the
  2026-07-06 lab pass confirmed `C.0123456789abcdef` reaches the RPC
  rather than being rejected), but since the lab has no enrolled
  clients, it has still not been confirmed against a real *existing*
  client's actual ID string.
- Artifact names used in `profiles/windows_ransomware_triage.yaml` and
  `profiles/linux_basic_triage.yaml` are illustrative and unverified.
- No Velociraptor-side ACL name list has been confirmed against a
  specific Velociraptor server version.
- **The real gRPC health check was confirmed against a real Velociraptor
  server on 2026-07-06** (see "Live lab validation" above) — `Check`
  succeeds with the lab's least-privilege `reader` identity, matching
  the upstream field names/numbers fetched from `Velocidex/velociraptor`
  on 2026-07-05. Not exercised live: the *failure* path against a real
  but unreachable/misconfigured server (wrong port, expired cert, CA
  mismatch) — only covered by fakes so far.
- **`ListClients` and `GetArtifacts` were confirmed against a real
  Velociraptor server on 2026-07-06**; `GetClient` was also confirmed
  live, and that pass caught and fixed a real bug in how this project
  handled its "unknown client ID" response (see "Live lab validation"
  above, `ErrClientNotFound`). Field names/numbers were originally
  fetched directly from `Velocidex/velociraptor`'s GitHub repo
  (`api/proto/api.proto`, `api/proto/clients.proto`,
  `api/proto/artifacts.proto`, `artifacts/proto/artifact.proto`, all
  read on 2026-07-05) and cross-checked against `api/clients.go` for
  `last_seen_at`'s unit (microseconds since epoch); `GetArtifacts`'s
  399-artifact real response and `max_rows` truncation are now
  confirmed working end to end. Still unconfirmed against a live
  server: `SearchClientsRequest.query`'s exact matching grammar (glob?
  regex? substring?) and per-client field-value correctness — the lab
  used for this pass had zero enrolled clients (see "Live lab
  validation" above and docs/lab-validation-plan.md's remaining
  unchecked Phase 2 items).
- `--profiles-dir`'s executable-relative fallback
  (`resolveProfilesDir`) is new and only tested via temp-directory
  fixtures in `cmd/agentic-velociraptor-mcp/main_test.go`, not against
  a packaged/installed binary layout.
- go.mod's `go` directive is 1.25.0, auto-selected when
  `modelcontextprotocol/go-sdk` was added in v0.1.0-alpha.1; confirm
  this is acceptable for your build/deploy environment.

## Immediate next step

The real gRPC calls (`Check`, `ListClients`, `GetClient`,
`GetArtifacts`) are now live-validated against a disposable lab
Velociraptor server (2026-07-06, see "Live lab validation" above and
docs/lab-validation-plan.md Phases 1 and 2) — that foundation is no
longer unvalidated. What's left before calling Phase 2 fully closed:
enroll at least one disposable client in a lab and re-run
`velo_search_clients`/`velo_get_client_info` against it, to confirm
real field-value mapping and `SearchClientsRequest.query`'s matching
grammar (both currently unverified due to the lab's empty client
inventory).

Next up per PROJECT_PLAN.md: v0.5.0, implementing the three
flow-visibility tools left over from the original v0.1.0 scope
(`velo_list_flows`, `velo_get_flow_status`, `velo_get_flow_results` —
each needs its own minimal `veloapi` RPC addition following the same
pattern as the `ListClients`/`GetClient`/`GetArtifacts` milestone: fetch
the exact upstream message/service shape first, add only the fields
actually used, generate with `buf`). After that, v0.6.0 covers
production hardening plus the real gRPC wiring this milestone's
collection/cancel/download tools are still missing (see "known
limitation" above).
