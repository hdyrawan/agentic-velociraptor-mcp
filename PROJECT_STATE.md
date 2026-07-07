# Project state

Last updated: 2026-07-07 (v0.10.2, live-lab validation of the collection/flow/hunt/IOC RPC groups).

## Current milestone

**v0.10.2 — Live-lab validation.** Validation-only: still exactly 28
tools, no raw VQL/generic query, no new write path, no weakened approval
gate/policy allowlist/audit behavior/fail-closed default. Ran the full
28-tool build against a disposable Docker-based Velociraptor 0.76.3 lab
(one server, one disposable Linux client, least-privilege `reader`/
`investigator` role-based API identities) via a real stdio subprocess
driven by MCP Inspector. See
[docs/live-validation-report-v0.10.2.md](docs/live-validation-report-v0.10.2.md)
for the full pass/fail table; summary:

- **First live confirmation of the collection and hunt RPC groups**
  that v0.10.1 still listed as "not yet live-validated": real
  `CollectArtifact`, `GetFlowDetails`, `GetTable`, `CreateHunt`,
  `EstimateHunt`, `CancelHunt`, and `ListHunts` all confirmed working
  end-to-end, including a hunt that was created, actually scheduled a
  real client, and produced a real hunt-driven flow.
- **Fixed one real bug**: `velo_compare_dfir_profiles`'s
  `common_artifacts` output duplicated an artifact once per profile that
  contained it instead of listing it once
  (`internal/mcpserver/tools_workflow.go`).
- **Found and documented two real bugs, not fixed this pass** (both
  correctness/capability gaps, not security-control weaknesses):
  `velo_get_flow_results`/`velo_get_hunt_results` silently report `empty`
  for artifacts with a named (non-default) Velociraptor source — notably
  `Generic.Client.Info`, used by nearly every DFIR profile — because
  `GetTable` needs a source-qualified name this project doesn't yet
  track; and the IOC-hunt artifact mapping
  (`System.Hash.Hunt`/etc.) is confirmed **not to exist** in any real
  Velociraptor catalog, so `velo_hunt_ioc_with_approval` cannot succeed
  against a real server as currently wired.
- Every approval-gated tool's one-shot consumption, fingerprint
  matching, and the explicit-`client_ids` hunt-scope pre-consume gate
  (all three affected tools) were reconfirmed live, including the
  approval-preserved-on-refusal behavior.
- See the report's "Limitations" section for what this pass could not
  confirm (a genuinely label-scoped hunt matching a labeled client — a
  lab-tooling limitation, not a code defect; no Windows client; no
  upload/download exercised against real evidence bytes).

## Previous milestone

**v0.10.1 — Stabilization: accurate docs, hunt-scope approval gate, CI.**
Preserves the 28-tool inventory, 46-profile curated DFIR catalog, and
every approval/policy/audit control unchanged. This milestone does not
add or remove any MCP tool, artifact, or capability; it corrects
documentation that had drifted behind the code (README/PROJECT_STATE/
PROJECT_PLAN/docs previously described v0.8.0-era "scaffolded" backend
paths and a 20-tool inventory, both stale since the GLM 5.2 hardening
pass and v0.10.0 below), closes a real approval-consumption gap in the
hunt-scope backend check, and adds CI. See CHANGELOG.md's `[Unreleased]`
v0.10.1 entry for the full list.

- **Pre-consume hunt-scope gate**: `velo_start_hunt_with_approval`,
  `velo_start_dfir_hunt_with_approval`, and `velo_hunt_ioc_with_approval`
  now refuse an explicit `client_ids` hunt scope in real backend mode
  *before* `gateAuditForWrite`/`consumeApproval`, not after. Previously,
  such a request passed every gate, consumed its one-shot approval, and
  only then failed inside `WriteClient.StartHunt` (real Velociraptor's
  typed hunt RPCs have no field for an explicit client-ID list — see
  `velociraptor.ErrHuntScopeClientIDsUnsupported`) — burning a human
  approval for a request that could never succeed. New
  `huntScopeBackendReady` (`internal/mcpserver/tools_hunts.go`) closes
  this; label- and all-clients-scoped hunts are unaffected.
- Docs (README, this file, PROJECT_PLAN.md, docs/tool-reference.md,
  docs/security-model.md, docs/lab-validation-plan.md) now describe the
  real gRPC backend wiring, 28-tool inventory, 46-profile catalog, and
  the client_ids limitation above accurately.
- Added `.github/workflows/ci.yml` (`go build`, `go vet`, `go test`,
  `gofmt` check, `git diff --check`); no secrets, no live Velociraptor
  dependency.

## Previous milestone (undocumented until now): GLM 5.2 hardening

Landed between the v0.8.0 tag and the v0.10.0 commit but never given its
own changelog/PROJECT_STATE entry — v0.10.1 above backfills that
documentation gap. Wired real typed gRPC RPCs for every remaining tool
group: flow list/status/results and collection/cancel
(`internal/velociraptor/grpcclient_flows.go`), flow uploads/download
(`grpcclient_uploads.go`), and hunt list/status/results/preview/start/
cancel (`grpcclient_hunts.go`), against a `veloapi` proto mirror extended
with typed flow/hunt/table/VFS/VQL message bindings. Also hardened the
approval store with a cross-process `flock` (fixing a race where a
concurrent `approve` CLI `Create` could resurrect a consumed approval),
made the audit sanitizer recurse into nested maps/slices/structs/pointers
instead of only a flat top-level map, and added audit log rotation
(`AuditConfig.MaxSizeBytes`/`MaxFiles`). This pass is also where
`velociraptor.ErrHuntScopeClientIDsUnsupported` was discovered and
documented: real Velociraptor's `HuntCondition` proto has no field for an
explicit client-ID list, only a label filter or "all clients" — the gap
v0.10.1's pre-consume gate above closes at the MCP-handler level. None of
this has been live-lab validated yet — see docs/lab-validation-plan.md.

## Previous milestone: v0.10.0 curated artifact catalog and DFIR profile expansion

Expanded curated DFIR coverage without touching the runtime execution
model: still exactly 28 MCP tools, no raw VQL, no arbitrary/agent-supplied
artifact names, no wildcard/prefix matching. Added `catalog/artifacts.yaml`
(the reviewed registry every profile artifact must appear in) and
`internal/dfir/catalog.go` (`LoadCatalog`, `ValidateProfileAgainstCatalog`,
enforced at `go test` time). Added 31 new catalog-verified profiles (46
total), every artifact confirmed present in a real Velociraptor lab
catalog (433 artifacts, 2026-07-07). The catalog is an authoring/test-time
control only — runtime collection is still gated solely by
`policy.allowed_artifacts`. See docs/dfir-profiles.md and CHANGELOG.md's
v0.10.0 entry.

## Backend wiring status (as of v0.10.2)

Every RPC group used by the 28 tools now has a reviewed typed gRPC
binding and unit tests against fake gRPC service stubs. Nothing here
exposes a generic VQL query path — the stable-core raw-VQL rule is
unchanged.

| Group | Status |
|---|---|
| Visibility (`health`, client search/info, artifact list/details) | Real gRPC; live-validated 2026-07-06. |
| Flow list/status/results | Real gRPC; live-validated 2026-07-07 for list/status and for default-source artifacts; **confirmed gap** for named-source artifacts (e.g. `Generic.Client.Info`) — see docs/live-validation-report-v0.10.2.md finding 2. |
| Collection start / DFIR profile collection / flow cancel | Real gRPC; backend capability checked before consuming approval; collection start live-validated 2026-07-07 (real `CollectArtifact`); flow cancel not yet live-validated. |
| Flow uploads list/metadata/download | Real gRPC; download backend capability checked before consuming approval; list confirmed honest-empty 2026-07-07 (no allowlisted artifact in that pass produced an upload); metadata/download not yet exercised against a real upload. |
| Hunts list/status/results/preview | Real gRPC; explicit `client_ids` scope refused before consuming approval (no typed RPC support); list/status/preview/all-clients-results live-validated 2026-07-07 end-to-end (real client scheduled and executed); label-scope matching not confirmed (lab-tooling limitation, not a code defect) and same named-source result gap as flow results. |
| Approved hunt start/cancel and IOC hunt | Real gRPC; same `client_ids` limitation; start (all-clients)/cancel live-validated 2026-07-07; **IOC hunt confirmed non-functional** against a real server — its artifact mapping (`System.Hash.Hunt`/etc.) does not exist in any real catalog. |

See [docs/live-validation-report-v0.10.2.md](docs/live-validation-report-v0.10.2.md)
for the full pass/fail detail behind this table.

Required follow-up before production: fix the named-source result-retrieval
gap and the IOC-hunt artifact mapping above, confirm label-based scoping
against a genuinely labeled client, validate against a Windows client and
a real file-producing upload, and keep `max_rows`, `max_result_bytes`,
`max_upload_bytes`, `max_hunt_clients`, `target_all`, cursor, audit, and
no-raw-VQL
invariants under test throughout.

## Previous milestone

**v0.7.0 — IOC hunting helper.** Adds the last planned tool
(`velo_hunt_ioc_with_approval`) on top of v0.6.0's hunt management
tools, and reviews v0.6.0's scaffolded backend paths for anything
safe/clear to implement for real (as opposed to gRPC hunt/collection
execution, which stays scaffolded — see "What does not exist yet").

- Added `velo_hunt_ioc_with_approval`: validates a `hash`, `ip`,
  `domain`, `process`, or `path` indicator (`internal/validation.ValidateIOC`;
  `Process`/`Path` are new this release), resolves it through a fixed
  `internal/vql` template to an allowlisted artifact + bound parameter
  (`internal/vql.Bind`, now fully implemented — see below), and starts
  a hunt via the same `velociraptor.HuntWriter.StartHunt` path
  `velo_start_hunt_with_approval` uses. Approval-gated via the new
  `approval.OperationHuntIOC` category, going through the same
  `verifyApproval/consumeApproval` fingerprint check as every other
  approval-gated tool.
- **Completed `internal/vql.Bind`**: previously always failed closed
  ("not yet implemented"); now deterministically maps each of 5
  `TemplateName`s (`ioc_hash_hunt`, `ioc_ip_hunt`, `ioc_domain_hunt`,
  and new `ioc_process_hunt`/`ioc_path_hunt`) to a fixed artifact name +
  parameter key, binding the caller's already-validated indicator value
  under that one key. This is real, tested, pure-Go logic with no gRPC
  call — the part of "review scaffolded backend paths" that was
  safe/clear to implement now. The artifact names themselves
  (`System.Hash.Hunt`, `System.IP.Hunt`, `System.Domain.Hunt`,
  `System.Process.Hunt`, `System.Path.Hunt`) remain illustrative/
  unverified against a live Velociraptor catalog — same caveat the
  pre-existing `ioc_hash_hunt`/`ioc_ip_hunt`/`ioc_domain_hunt` DFIR
  profiles already carried (see docs/dfir-profiles.md). Real hunt-start
  gRPC execution was judged unclear/risky without live-lab validation
  and stays scaffolded, unchanged from v0.6.0.
- **Fixed a v0.6.0 confused-deputy gap**: the three hunt-write tools
  (`velo_start_hunt_with_approval`, `velo_start_dfir_hunt_with_approval`,
  `velo_cancel_hunt_with_approval`) previously used a bespoke
  `checkHuntApproval` helper that only checked "is this approval ID
  approved and unconsumed," never that the approved request's
  operation/case/artifact/scope matched the actual call — any valid
  unconsumed approval could start/cancel a *different* hunt than
  approved. Replaced with `verifyApproval/consumeApproval`, the same
  fingerprint-matching path v0.4.0's collection tools use. Extended
  `approval.Request`/`RequestFingerprint` with `ClientIDs`/`Label`/
  `TargetAll` so a hunt's multi-client scope (not just `ClientID`) is
  now part of what an approval pins down. Also fixed approvals being
  consumed before the read-only/write-client gates were checked.
- **Fixed a v0.6.0 annotations gap**: the same three hunt-write tools
  were registered with `Annotations: nil`, and
  `TestNewRegisteredToolsAreNonDestructiveAndClosedWorld` had been
  weakened in the same PR to skip nil-annotation tools rather than
  flag it. Both fixed: all three (plus the new IOC tool) now use
  `writeAnnotations(...)`, and the test has no exception.
- **Fixed a v0.6.0 build break**: `tools_hunts.go` redeclared six
  helpers already defined in `tools_flows.go`
  (`nextOffsetCursor`/`boundRowsByLimitAndBytes`/`boundToolLimit`/
  `configuredMaxRows`/`configuredMaxResultBytes`/`minInt`); the branch
  as committed did not compile. Removed the duplicates.
- `agentic-velociraptor-mcp approve` CLI gained `--hunt-client-id`
  (repeatable), `--label`, `--all`, and `--hunt-id` flags, and
  `start_hunt`/`start_dfir_hunt`/`cancel_hunt`/`hunt_ioc` operation
  support — previously an operator had no way to construct an approval
  for any hunt operation's scope at all.
- New `audit.Event.IOCKind`/`IOCValue` fields (additive).
- Callable tool inventory is now exactly 28: 5 visibility + 3 DFIR
  profile + 3 DFIR workflow + 3 flow/result + 3 flow uploads +
  3 collection + 7 hunt management + 1 IOC helper.

## Previous milestone

**v0.6.0 — Hunt management schema/safety scaffold.** Backfill of the
original v0.4.0/v0.5.0 hunt-management scope from the PROJECT_PLAN.md
roadmap. All seven MCP tool handlers were implemented and registered,
but **real gRPC hunt execution was NOT implemented** — write operations
invoke `Deps.WriteClient` (currently `placeholderClient` →
`ErrNotImplemented`). Two issues from this milestone (the approval
fingerprint gap and the missing tool annotations) were found and fixed
while landing it; see "Current milestone" above.

- Implemented seven hunt management tool handlers:
  `velo_preview_hunt_scope` (RO, blocks `target_all` by default),
  `velo_start_hunt_with_approval` (approval-gated, allowlisted artifacts,
  enforces `max_hunt_clients`),
  `velo_start_dfir_hunt_with_approval` (approval-gated, profile
  allowlist, validates via `dfir.ValidateProfile`),
  `velo_list_hunts` (RO, cursor pagination),
  `velo_get_hunt_status` (RO, not-found sentinel),
  `velo_get_hunt_results` (RO, bounded by max_rows/max_result_bytes,
  cursor pagination),
  `velo_cancel_hunt_with_approval` (approval-gated).
- Approval-gated tools enforce four independent gates before any
  Velociraptor call: fingerprint-matched approval verification (fixed in
  v0.7.0 above), `policy.Engine` mode check (read-only mode blocks
  writes), scope validation (`validation.ValidateHuntScope`), and
  artifact/profile allowlist enforcement plus `max_hunt_clients` cap.
- Read-only hunt tools (preview, list, status, results) branch on
  `Deps.VelociraptorReadMode` (mock/real) like the v0.1.0 visibility
  tools. In real mode, read-only tools call `Deps.ReadClient` which also
  returns `ErrNotImplemented` for hunt methods — so they follow the
  mock/real branching pattern but still cannot execute against a real
  server.
- New errors: `velociraptor.ErrHuntNotFound`, `velociraptor.ErrFlowNotFound`,
  `velociraptor.ErrTargetAllDisabled`.
- Callable tool inventory was 27 before v0.7.0 above added the IOC tool.

## Older milestone

**v0.4.0 — Controlled single-client collection pilot.** Complete. This
is this project's **first write-capable Velociraptor feature**, and it
is deliberately a controlled pilot, not unrestricted write access: no
hunts, no multi-client collection, no raw VQL, no destructive action.
Implemented independently of, and merged after, v0.5.0's read-only
flow/result backfill (see below); this branch was rebased onto that work
rather than the two conflicting.

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

### v0.5.0 — Read-only flow/result backfill (precedes v0.4.0)

Complete at the MCP handler and read-client interface layer. This
milestone closes the original v0.1.0 tool-surface gap without adding
collection execution, hunt start or cancel, flow cancel, downloads,
write identity use, client/server mutation, or raw VQL exposure.

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
- **New in v0.7.0**: `internal/mcpserver/tools_ioc.go`, registering
  `velo_hunt_ioc_with_approval`. `internal/vql.Bind` (`render.go`) now
  implements the template → (artifact, param key) mapping for all 5 IOC
  templates instead of always failing closed.
  `internal/validation/ioc.go` adds `Process`/`Path`/`ValidateIOC`.
  `approval.Request` gains `ClientIDs`/`Label`/`TargetAll`, folded into
  `RequestFingerprint`. `audit.Event` gains `IOCKind`/`IOCValue`.
  `cmd/agentic-velociraptor-mcp/main.go`'s `approve` subcommand gains
  hunt-scope flags and `start_hunt`/`start_dfir_hunt`/`cancel_hunt`/
  `hunt_ioc` operation support. `internal/mcpserver/tools_ioc_test.go`
  covers IOC kind/value validation (all 5 kinds), policy gates
  (read-only, no-approval, target_all, max_hunt_clients), fingerprint
  mismatch (including a cross-tool case proving an IOC approval cannot
  authorize a plain hunt start), the approved fake-client path, and the
  scaffolded real-mode honest-error path.
- **New in v0.6.0**: `internal/mcpserver/tools_hunts.go`, registering
  seven hunt management tools. `internal/velociraptor/hunts.go` adds the
  `HuntReader`/`HuntWriter` interfaces with `ErrHuntNotFound` and
  `ErrTargetAllDisabled`. `internal/velociraptor/flows.go` adds
  `ErrFlowNotFound`. `internal/mcpserver/tools_hunts_test.go` covers all
  7 tools with test cases including approval gating (fingerprint-matched
  as of v0.7.0's fix above), scope validation, allowlist enforcement,
  pagination, and no-raw-VQL guarantees.
- **New in v0.3.0**: `internal/mcpserver/tools_workflow.go`, registering
  three read-only local workflow helpers: `velo_plan_dfir_triage`,
  `velo_compare_dfir_profiles`, and `velo_find_profiles_by_artifact`.
- **New in v0.2.0**: `internal/response` (the `Result`/`Status` envelope)
  and `velociraptor.ErrArtifactNotFound`.
- Go module `github.com/hdyrawan/agentic-velociraptor-mcp` (go 1.25).
- Dependencies: `gopkg.in/yaml.v3`, `github.com/modelcontextprotocol/go-sdk`
  v1.6.1 (MCP protocol), `google.golang.org/grpc` v1.82.0 and
  `google.golang.org/protobuf` v1.36.11, plus their transitive deps. No other
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
- `internal/mcpserver`: 28 registered tools:
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
  `velo_download_flow_upload_with_approval`,
  `velo_preview_hunt_scope`, `velo_start_hunt_with_approval`,
  `velo_start_dfir_hunt_with_approval`, `velo_list_hunts`,
  `velo_get_hunt_status`, `velo_get_hunt_results`,
  `velo_cancel_hunt_with_approval`, `velo_hunt_ioc_with_approval`. All
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
  All 28 planned tools are now registered and callable; the
  exact-tool-inventory test expects 28
  (`TestNewRegistersExactlyTwentyEightTools`).
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

As of v0.10.1, every RPC group below has a real typed gRPC binding (see
"Backend wiring status" above) — the gaps that remain are live-lab
validation and a small number of deliberately out-of-scope items:

- **Live-lab validation of collection/flow/upload/hunt/IOC RPCs against a
  real Velociraptor server.** All of it is unit-tested against fake gRPC
  service stubs, not yet exercised against a live deployment with
  enrolled endpoints. See docs/lab-validation-plan.md's unchecked Phase
  4-7 items.
- **Explicit `client_ids` hunt scope in real mode**, by design: real
  Velociraptor's `HuntCondition` proto has no field for an explicit
  client-ID list (only label or all-clients). `velo_preview_hunt_scope`,
  `velo_start_hunt_with_approval`, `velo_start_dfir_hunt_with_approval`,
  and `velo_hunt_ioc_with_approval` all refuse this scope in real mode
  with a structured error (`velociraptor.ErrHuntScopeClientIDsUnsupported`),
  and the three approval-gated ones do so before consuming the approval.
  A workaround exists upstream (label the target clients, run a
  label-scoped hunt, unlabel them) but was judged out of scope without
  live-lab validation — it mutates client state beyond what a hunt-start
  operation should do.
- Confirmation that `System.Hash.Hunt`/`System.IP.Hunt`/
  `System.Domain.Hunt`/`System.Process.Hunt`/`System.Path.Hunt` (the
  artifact names `vql.Bind` resolves IOC templates to) exist in any real
  Velociraptor artifact catalog — illustrative/unverified, same caveat
  as the pre-existing IOC DFIR profiles; see docs/dfir-profiles.md and
  docs/lab-validation-plan.md.
- Any raw/generic VQL query RPC — intentionally, permanently out of
  scope for the stable core (see docs/security-model.md).
- HTTP/SSE/streamable HTTP transport — intentionally out of scope.
- Docker image hardening, rate limiting, further integration tests (see
  PROJECT_PLAN.md's production-hardening entry).

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

The 28-tool stable-core target is complete (PROJECT_PLAN.md), and v0.10.2
(see "Current milestone" above and
docs/live-validation-report-v0.10.2.md) live-validated the collection and
hunt RPC groups end-to-end against a disposable lab. The next steps are:

1. **Fix the named-source result-retrieval gap** — `velo_get_flow_results`/
   `velo_get_hunt_results` cannot currently retrieve rows for
   `Generic.Client.Info` or any other multi-source artifact (v0.10.2
   finding 2). Requires adding source-name (not query-body) metadata to
   `veloapi.Artifact` and threading it through both RPC call sites.
2. **Replace the IOC-hunt artifact mapping** — `System.Hash.Hunt`/etc.
   confirmed not to exist in any real Velociraptor catalog (v0.10.2
   finding 3); `internal/vql.Bind`'s template targets need real,
   catalog-verified replacements before `velo_hunt_ioc_with_approval` can
   work against a real server.
3. Confirm label-based hunt/collection scoping against a client with a
   verified, persistent label (not demonstrated in v0.10.2 — see its
   "Limitations" section), validate against a Windows client, and
   exercise the upload/download path against a real file-producing
   collection.
4. The remaining unchecked items in docs/lab-validation-plan.md (Phase
   5's `velo_cancel_flow_with_approval`; Phase 8's adversarial testing).

Do not point this project at a production Velociraptor deployment until
that validation is complete — see docs/production-deployment.md.
