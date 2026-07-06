# Project state

Last updated: 2026-07-06 (v0.1.0, live lab validation pass).

## Current milestone

**v0.1.0 — Read-only Velociraptor visibility.** Complete for the client
and artifact catalog tools, and now live-validated against a real
disposable Velociraptor lab (see "Live lab validation" below and
docs/lab-validation-plan.md's Phase 1/Phase 2 sections for exact
results). `velo_list_flows`/`velo_get_flow_status`/`velo_get_flow_results`
(also originally scoped to v0.1.0 in PROJECT_PLAN.md) are deferred — no
RPC exists yet for them. Next: either add those three flow-visibility
tools, or move on to v0.2.0 (controlled single-client collection), per
user direction.

## Live lab validation (2026-07-06)

Validated the 8-tool read-only surface against a real Velociraptor lab
(container `velociraptor-lab`, image `wlambert/velociraptor:latest`,
**Velociraptor version 0.76.3**), using the lab's pre-generated
**`mcp-reader` reader API identity** (role `reader`, not
`administrator`; the investigator identity was never used). Driven over
a real stdio subprocess (`mcp.CommandTransport` against the actual built
binary), not just unit tests against fakes.

- **Callable inventory: still exactly 8** — `ListTools` against the live
  subprocess returned the same 8 tools as
  `internal/mcpserver/server_test.go`'s exact-inventory test. No new
  tool was added, no flow tools were added.
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
- `internal/mcpserver`: 8 registered tools (up from 4):
  `velo_health_check`, `velo_search_clients`, `velo_get_client_info`,
  `velo_list_artifact_names`, `velo_get_artifact_details`,
  `velo_list_dfir_profiles`, `velo_get_dfir_profile`,
  `velo_validate_dfir_profile`. All five visibility tools share
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
  The other 16 planned tools remain unregistered `ToolSpec` metadata;
  the exact-tool-inventory test now expects 8, and a new
  `TestNewNeverRegistersUnsafeTools` test guards against a
  collect/hunt/download/cancel/vql-named tool ever becoming callable.
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
- Tests: `internal/velociraptor/grpcclient_test.go` gained
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

- `velo_list_flows`, `velo_get_flow_status`, `velo_get_flow_results` —
  originally scoped to v0.1.0 in PROJECT_PLAN.md but deferred: no
  `veloapi` RPC exists yet for flow listing/status/results, and none was
  added in this pass. Revisit as a follow-up before calling v0.1.0 fully
  done against the original plan, or explicitly re-scope them into
  v0.2.0 in PROJECT_PLAN.md.
- Any of the other 16 stable-core tools as callable MCP tools
  (collection, hunts, uploads, IOC).
- Any RPC beyond `Check`/`ListClients`/`GetClient`/`GetArtifacts` — no
  `Query`, no flow/hunt RPCs, all still `ErrNotImplemented` on
  `grpcClient` (delegated to the embedded placeholder).
- Any approval mechanism implementation (types only; `Deps.Approvals`
  still `nil`).
- Any audit sanitizer implementation beyond a flat top-level map
  redactor.
- Any write-capable Velociraptor client — `WriteClient` is always the
  mock placeholder; `write_api_config_path` is not read by any code.
- HTTP/SSE/streamable HTTP transport — intentionally out of scope.
- Real-server validation of `velo_search_clients`/`velo_get_client_info`
  against actual enrolled endpoint clients — the 2026-07-06 lab pass (see
  above) had zero enrolled clients, so field-value correctness
  (hostname/OS/last-seen/labels/MAC addresses) and
  `SearchClientsRequest.query`'s real matching grammar remain unverified.
  The next person with access to a lab with at least one enrolled client
  should run through docs/lab-validation-plan.md's remaining unchecked
  Phase 2 items.
- Docker image, rate limiting, further integration tests (v0.5.0
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

Otherwise, either (a) implement the three flow-visibility tools left
over from the original v0.1.0 scope (`velo_list_flows`,
`velo_get_flow_status`, `velo_get_flow_results` — each needs its own
minimal `veloapi` RPC addition following the same pattern as this
milestone's `ListClients`/`GetClient`/`GetArtifacts`: fetch the exact
upstream message/service shape first, add only the fields actually
used, generate with `buf`), or (b) move on to v0.2.0 (controlled
single-client collection) and explicitly re-scope the flow tools into
it in PROJECT_PLAN.md.
