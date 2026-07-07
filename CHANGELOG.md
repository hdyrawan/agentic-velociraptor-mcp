# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versioning will follow [SemVer](https://semver.org/) once tagged
releases begin.

## [Unreleased]

### Added — v0.10.0 curated artifact catalog and DFIR profile expansion

Expands curated DFIR coverage without touching the runtime execution
model: no new MCP tools (still exactly 28), no raw VQL, no
arbitrary/agent-supplied artifact names, no wildcard/prefix matching, and
no change to `allow_raw_vql`/`allow_list_all_artifacts` (both stay
false).

- Added `catalog/artifacts.yaml`, a curated artifact catalog: the single
  reviewed registry of every artifact name any profile may reference,
  with metadata (`name`, `target_os`, `category`, `risk_level`,
  `requires_approval`, `sensitivity`, `verified`, `notes`). It is an
  authoring/test-time control only — runtime collection is still gated
  solely by `policy.allowed_artifacts`; the catalog never widens or
  bypasses that allowlist.
- Added `internal/dfir/catalog.go` (`LoadCatalog`, `Catalog`,
  `ValidateProfileAgainstCatalog`) and `internal/dfir/catalog_test.go`.
  New tests enforce, at `go test` time, that every profile artifact
  exists in the catalog and that any profile bundling a
  `requires_approval: true` artifact is itself `requires_approval: true`.
- Added 31 curated, catalog-verified DFIR profiles (46 total): Windows
  inventory/PowerShell/scheduled-task/WMI/service persistence, execution
  evidence, authentication events, user activity, network connections,
  filesystem timeline; five Windows browser sub-profiles
  (history/downloads/extensions/cookies/cache); Linux
  inventory/process/network/auth/SSH-trust/privilege/shell-history/cron/
  systemd/packages/containers; and cross-platform
  identity/process/network/IOC-context/local-hashes. Every new profile
  references only artifacts confirmed present in a real Velociraptor lab
  catalog (433 artifacts, 2026-07-07). High-risk/privacy-sensitive
  profiles are `requires_approval: true`; SSH private keys are
  deliberately never collected.
- Pre-existing illustrative artifact names (e.g. `System.Hash.Hunt`,
  `Windows.Browser.History`) are catalogued as `verified: false` with a
  note pointing at the nearest real artifact; no new profile is built on
  them.
- Docs (`docs/dfir-profiles.md`) updated with the 46-profile catalog,
  the artifact-catalog model, approval rationale, and verified-vs-candidate
  status. The example config gains commented-out optional allowlist
  blocks only — no enabled-by-default artifacts or profiles.

## [0.8.0] - 2026-07-06

### Security — code review (Fable 5) fixes

- **Approval fingerprint hardened against delimiter injection (S1).**
  `approval.RequestFingerprint` now uses an injective, length-prefixed
  field encoding instead of newline-delimited text, so a field or
  parameter value embedding another field's serialized form can no
  longer collide with a genuinely different request (previously a
  parameter map `{k: "v\nparam:k2=v2"}` fingerprinted identically to
  `{k: "v", k2: "v2"}`, letting an approval execute with different bound
  parameters than approved). Regression tests in
  `internal/approval/hash_test.go`. Pending approvals created by earlier
  builds will no longer fingerprint-match and must be re-created.
- **Hunt/IOC write-path validation brought up to collection-path parity
  (S2).** New `validation.HuntID` (`H.` + 4-128 URL-safe chars, enforced
  by `velo_cancel_hunt_with_approval`) and `validation.HuntLabel`
  (allowlisted charset, enforced by `validation.ValidateHuntScope` for
  every label-scoped hunt/preview). `validateHuntWriteInput` now
  validates `case_id`, `reason`, `requester` (previously unchecked), and
  `approval_id` with the same `internal/validation` rules the collection
  tools use. `case_id`, `requester`, and collection parameter keys/values
  reject embedded newlines (multi-line `reason` remains legal).
- **Hunt/IOC policy checks fail closed (S3).** The artifact/profile
  allowlist checks in `velo_start_hunt_with_approval`,
  `velo_start_dfir_hunt_with_approval`, and `velo_hunt_ioc_with_approval`
  now deny when `deps.Policy` is nil (previously they skipped the
  allowlist entirely in that state; unreachable in practice but fragile).
- **Approval-gated writes fail closed on a broken audit sink (S4).**
  Every approval-gated write handler durably records an
  `audit.OutcomeAttempt` event after all policy/approval/backend gates
  pass and *before* consuming its approval; if that event cannot be
  persisted the operation is refused with the approval intact, so no
  endpoint-facing write can execute unaudited. Non-gating audit writes
  now fall back to a structured stderr line instead of being silently
  discarded.
- **`approve` CLI supports `hunt_ioc` (Q1).** `velo_hunt_ioc_with_approval`
  was previously unusable end-to-end: the CLI rejected
  `--operation hunt_ioc`, so no approval for it could ever exist. The CLI
  now accepts `--ioc-kind`/`--ioc-value` and builds the request through
  the same exported path the handler fingerprints
  (`mcpserver.BuildHuntIOCApprovalRequest`: `ValidateIOC` → fixed
  template → `vql.Bind`), guaranteeing CLI-created approvals verify
  byte-for-byte. Regression tests cover the CLI create path and the
  CLI→handler round trip.

### Changed — backend wiring review and approval consume ordering

- Preserved the v0.7.0 28-tool MCP inventory exactly; no tools were added or removed.
- Reviewed every scaffolded flow, collection, upload, hunt, and IOC backend path. Because the current hand-authored `veloapi` mirror only includes `Check`, `ListClients`, `GetClient`, and `GetArtifacts`, no additional real Velociraptor write/read RPCs could be wired safely without introducing a generic/raw VQL path.
- Added explicit backend-capability checks for approval-gated operations. Policy/input/scope/allowlist/backend gates now pass before `approval.Store.Consume` is called; scaffolded or missing write backends return structured `backend_not_implemented`/`error` results and preserve the one-shot approval.
- Kept real gRPC visibility paths unchanged: health, client search/info, and artifact list/details remain the only real Velociraptor RPC-backed operations in this build.
- Added regression coverage proving scaffolded backend gaps do not consume approvals.

## [0.7.0] - 2026-07-06

### Added — v0.7.0 IOC hunting helper

Adds the last planned tool from the original stable-core target: a
single fixed-template IOC hunting helper, built entirely on v0.6.0's
hunt approval/scope/audit machinery.

- Added `velo_hunt_ioc_with_approval`: validates a `hash`, `ip`,
  `domain`, `process`, or `path` indicator (`internal/validation.ValidateIOC`;
  `Process`/`Path` are new validators added this release), resolves it
  through a fixed `internal/vql` template to an allowlisted artifact +
  bound parameter (never raw VQL, never string interpolation), then
  starts a hunt via the same `velociraptor.HuntWriter.StartHunt` path
  `velo_start_hunt_with_approval` uses.
- Completed `internal/vql.Bind`: a deterministic, pure-Go
  template-name → (artifact, parameter key) mapping for all 5 IOC
  templates (`ioc_hash_hunt`, `ioc_ip_hunt`, `ioc_domain_hunt`, plus new
  `ioc_process_hunt`/`ioc_path_hunt`). This mapping is real and tested;
  the artifact names themselves remain illustrative/unverified against a
  live Velociraptor catalog, same caveat as the pre-existing IOC DFIR
  profiles. Real hunt-start gRPC execution stays scaffolded
  (`velociraptor.HuntWriter.StartHunt` still returns `ErrNotImplemented`
  on `grpcClient`).
- New `approval.OperationHuntIOC` operation category; every approval-gated
  IOC hunt goes through the same `verifyApproval/consumeApproval`
  fingerprint-matching path as every other approval-gated tool (no
  bespoke, weaker approval check). `approval.Request`/`RequestFingerprint`
  gained `ClientIDs`/`Label`/`TargetAll` fields so a hunt's multi-client
  scope (not just a single `ClientID`) is part of what an approval pins
  down — this also closes a gap the v0.6.0 hunt tools had (see Fixed
  below).
- `agentic-velociraptor-mcp approve` CLI gained `--hunt-client-id`
  (repeatable), `--label`, `--all`, and `--hunt-id` flags so an operator
  can actually construct an approval for `start_hunt`/`start_dfir_hunt`/
  `cancel_hunt`/`hunt_ioc`'s multi-client scope, not just single-client
  operations.
- Callable tool inventory is now exactly 28 (27 from v0.6.0 plus the one
  new IOC tool).
- New `audit.Event.IOCKind`/`IOCValue` fields (additive, non-breaking)
  record which indicator an IOC hunt targeted.

### Fixed — v0.6.0 hunt approval fingerprint bypass and build break

Two issues found in code review of the v0.6.0 branch before this release,
fixed as part of landing it:

- v0.6.0's hunt-start/cancel handlers used a bespoke `checkHuntApproval`
  helper that only checked "is this approval ID approved and unconsumed,"
  never that the approved request's operation/case/artifact/scope
  actually matched the call being made. Any valid, unconsumed approval
  could be replayed to start or cancel a *different* hunt than a human
  approved. Replaced with the same `verifyApproval/consumeApproval` fingerprint
  check every other approval-gated tool uses, and extended
  `approval.Request` with hunt-scope fields (see above) so scope is
  covered too. This also fixed approvals being consumed before the
  read-only/write-client gates were checked.
- `velo_start_hunt_with_approval`/`velo_start_dfir_hunt_with_approval`/
  `velo_cancel_hunt_with_approval` were registered with no MCP tool
  annotations at all (`Annotations: nil`), and the regression test that
  should have caught this
  (`TestNewRegisteredToolsAreNonDestructiveAndClosedWorld`) had been
  weakened in the same PR to skip nil-annotation tools. Both fixed: all
  three now use `writeAnnotations(...)` like every other write tool, and
  the test no longer has an exception.
- `tools_hunts.go` redeclared six helpers already defined in
  `tools_flows.go`, so the v0.6.0 branch as committed did not compile.
  Removed the duplicates.

## [0.5.0] - 2026-07-06

### Added — v0.5.0 read-only flow/result backfill

Re-scoped to backfill the original v0.1.0 read-only flow/result surface
without adding collection execution, hunt execution, cancellation,
downloads, write-identity use, or raw VQL exposure.

- Added three callable read-only MCP tools: `velo_list_flows`,
  `velo_get_flow_status`, and `velo_get_flow_results`.
- All three validate `client_id`/`flow_id` before any backend call, embed
  the shared `internal/response.Result` status envelope, honestly report
  mock mode, and audit every invocation.
- `velo_get_flow_results` enforces `velociraptor.max_rows` and
  `velociraptor.max_result_bytes`, reports `truncated`, returns
  `next_cursor` when partial data is returned, and records row/byte counts
  in audit events.
- Callable tool inventory is now exactly 14, all read-only. Upload,
  collection, hunt, cancel, download, and IOC execution ToolSpec entries
  remain unregistered metadata only.
- Tests added for flow success, empty, not_found, invalid input,
  row/byte-limit enforcement, audit events, MCP-session coverage, and the
  exact 14-tool inventory.
- Docs updated: README, PROJECT_PLAN, PROJECT_STATE, tool reference,
  security model, lab validation plan, and CLI help/version text.

## [0.4.0] - 2026-07-06

### Added — v0.4.0 controlled single-client collection pilot

This project's **first write-capable Velociraptor feature**. It is a
controlled pilot, not unrestricted write access: still single-client per
call, still no hunts, still no raw VQL, still no destructive action.

- Added six new callable MCP tools:
  `velo_collect_artifact_with_approval`,
  `velo_collect_dfir_profile_with_approval`,
  `velo_cancel_flow_with_approval` (`internal/mcpserver/tools_collection.go`),
  `velo_list_flow_uploads`, `velo_get_flow_upload_metadata` (read-only),
  and `velo_download_flow_upload_with_approval`
  (`internal/mcpserver/tools_flows.go`). Callable tool inventory
  increases from 14 (after v0.5.0's read-only flow/result backfill,
  merged separately and reconciled by rebase) to 20.
- Implemented `internal/approval.FileStore`, the first real
  `approval.Store`: JSON-file-backed, re-reads on every call, and adds
  `Requester`/`Parameters`/`FlowID`/`UploadName` to `approval.Request`
  plus a `Status` type (`Consumed`/`Expired` lifecycle) and `Consume`
  method to the `Store` interface. `approval.RequestFingerprint` now
  covers parameters/flow_id/upload_name in addition to the original
  operation/case/client/artifact/profile/hunt fields.
- Added a non-MCP `agentic-velociraptor-mcp approve` CLI subcommand: the
  only way to create and decide an `approval.Request`. It is never
  called by the MCP server and has no MCP tool equivalent, so no MCP
  client — including an LLM driving tool calls — can self-approve its
  own request.
- Every approval-gated tool call requires `case_id`, `reason`,
  `requester`, its target, and an `approval_reference`, verified by the
  new `mcpserver.verifyApproval/consumeApproval`: the reference must resolve to
  an approved, unconsumed, unexpired record whose fingerprint exactly
  matches the call, or the tool refuses (typed `not_found`/`error`
  responses, audited `blocked`) without ever calling Velociraptor.
  As of v0.8.0, approval is consumed only after backend-capability gates pass and immediately before the Velociraptor call, so a single human
  decision authorizes at most one execution attempt.
- The whole write pilot is off unless both `policy.mode: controlled` and
  the new `approval.store_path` config are set
  (`mcpserver.writePilotEnabled`); `velo_download_flow_upload_with_approval`
  additionally requires the new `velociraptor.download_dir` setting and
  never returns raw evidence bytes inline — only a local path, size, and
  SHA-256 after writing them to that directory under a filename derived
  only from already-validated client/flow IDs, never the caller-supplied
  upload name.
- `cmd/agentic-velociraptor-mcp` now also constructs a real `WriteClient`/
  `VelociraptorWriteMode` from `velociraptor.write_api_config_path`,
  mirroring the existing read-client wiring.
- **Known limitation**: the hand-authored `veloapi` proto mirror does not
  yet wire real gRPC bindings for `CollectArtifact`/`CancelFlow`/upload
  RPCs, so a real (non-mock) write client currently reports
  `velociraptor.ErrNotImplemented` honestly rather than fabricating
  success. All policy/approval/audit control-flow is implemented and
  tested against fake `velociraptor.Client` implementations; see
  `internal/mcpserver/tools_collection_test.go` and `tools_flows_test.go`.
  Real RPC wiring is deferred to v0.6.0.
- Existing 14 read-only tools (v0.1.0-v0.5.0), the v0.2.0
  `response.Result` envelope, and the v0.3.0 workflow tools are all
  preserved unchanged.
- Tests added: `internal/approval/filestore_test.go` (Store lifecycle,
  fingerprinting), `internal/validation/request_test.go` (new
  CaseID/Reason/Requester/ApprovalReference/FlowID/UploadName/
  CollectionParameters validators), `internal/config/config_test.go`
  (approval config validation), `internal/mcpserver/tools_collection_test.go`
  and `tools_flows_test.go` (disabled mode, invalid input, missing/denied/
  expired/consumed/mismatched approval, approved fake execution, audit
  fields, response statuses), and `cmd/agentic-velociraptor-mcp/main_test.go`
  (approve CLI, write-client/approval-store wiring). `server_test.go`
  updated to the 20-tool inventory (rebased onto v0.5.0's 14-tool
  read-only baseline).
- Docs updated: README, CHANGELOG, PROJECT_PLAN (realigned with the
  original roadmap; production hardening renumbered to v0.6.0),
  PROJECT_STATE, docs/tool-reference.md, docs/approval-flow.md (from
  draft design to implemented workflow), docs/security-model.md.

## [0.3.0] - 2026-07-06

### Added — v0.3.0 read-only DFIR workflow expansion

Re-scoped by explicit user direction from the original v0.3.0 hunt
management plan. This release remains strictly read-only: it does not
execute collections, start or cancel hunts, download evidence, mutate
clients, use the write API identity, or expose raw VQL.

- Added three callable MCP workflow tools, all local/read-only over the
  loaded DFIR profile registry and policy allowlists:
  `velo_plan_dfir_triage`, `velo_compare_dfir_profiles`, and
  `velo_find_profiles_by_artifact`.
- All three new tool outputs embed `internal/response.Result`, preserving
  the v0.2.0 status vocabulary (`success`, `empty`, `not_found`,
  `error`). Existing visibility/profile tool fields were not removed or
  renamed.
- Callable tool inventory is now exactly 11, all read-only. The planned
  flow, collection, hunt execution, cancel, download, and IOC execution
  ToolSpec entries remain unregistered metadata only.
- Tests added for workflow success, empty, not_found, and blocked invalid
  input paths, plus MCP-session coverage for the three new tools and an
  exact 11-tool inventory assertion.
- Docs updated: README, PROJECT_PLAN, PROJECT_STATE, tool reference,
  security model, and CLI help/version text.

## [0.2.0] - 2026-07-06

### Added — v0.2.0 core response validation and consistent response contracts

Re-scoped by explicit user direction from PROJECT_PLAN.md's original
v0.2.0 plan ("controlled single-client collection" — deferred,
unimplemented, no longer assigned to a specific version). This
milestone added no write-capable Velociraptor action; the callable tool
inventory is unchanged (still the same 8 read-only tools as v0.1.0).

- New `internal/response` package: a small `Result` type (`Status` +
  `Message`) with a normalized status vocabulary (`success`, `empty`,
  `not_found`, `error`) and a `StatusForCount` helper, meant to be
  embedded into tool response types instead of each handler inventing
  its own ad-hoc combination of `mode`/`message` fields.
- Embedded `response.Result` into `SearchClientsOutput`,
  `GetClientInfoOutput`, `ListArtifactNamesOutput`, and
  `GetArtifactDetailsOutput` (`internal/mcpserver/tools_visibility.go`),
  adding a top-level `status` field to all four visibility tools'
  responses. Additive to the wire shape only — no existing field was
  renamed or removed, and mock-mode/real-mode/allowlist behavior is
  unchanged. `velo_health_check`'s own pre-existing `status` field
  (`"ok"`/`"error"`, from v0.1.0-alpha.2) was deliberately left as-is:
  migrating it to the new vocabulary would have been a breaking
  wire-value change for no functional gain.
- Fixed a real gap: `velo_get_client_info` and
  `velo_get_artifact_details` previously reported a genuine "no such
  client"/"no such artifact" lookup exactly the same way as a generic
  connectivity/RPC failure — same `mode: "real"`, only the free-text
  `message` differed, with nothing machine-readable to branch on. Both
  now report a distinct `status: "not_found"`. Added
  `velociraptor.ErrArtifactNotFound` (`internal/velociraptor/artifacts.go`),
  mirroring the existing `velociraptor.ErrClientNotFound` sentinel from
  v0.1.0's live lab pass, and wrapped it into
  `grpcClient.GetArtifactDetails`'s not-found return
  (`internal/velociraptor/grpcclient.go`).
- Tests: `internal/response` gained `response_test.go` (every
  constructor sets its documented status; `StatusForCount`'s 0/1
  boundary). `internal/mcpserver/tools_visibility_test.go` gained
  `Status` assertions on existing success/mock/error cases plus new
  tests: empty-result status for `velo_search_clients` and
  `velo_list_artifact_names`, a misconfigured-`ReadClient` status-error
  case for both, and not-found tests for `velo_get_client_info` and
  `velo_get_artifact_details` (previously untested at the handler
  level). `internal/velociraptor/grpcclient_test.go`'s existing
  `TestGRPCClientGetArtifactDetailsNotFound` now asserts
  `errors.Is(err, ErrArtifactNotFound)`, matching the equivalent
  client-not-found test.
- Docs: PROJECT_PLAN.md (v0.2.0 section re-scoped, with a note on where
  the original collection scope went), PROJECT_STATE.md ("Current
  milestone" and "What exists" updated), docs/tool-reference.md (new
  response-envelope note; collection/upload tool rows marked
  "unscheduled"), docs/security-model.md ("Evidence honesty" section
  extended with the v0.2.0 envelope and not-found details).

## [0.1.0] - 2026-07-06

### Fixed — live lab validation of v0.1.0 read-only visibility (2026-07-06)

- Validated the 8-tool read-only surface against a real disposable
  Velociraptor lab (version 0.76.3) using its generated least-privilege
  `reader` API identity, over a real stdio subprocess — not just unit
  tests against fakes. See docs/lab-validation-plan.md's Phase 1/Phase 2
  sections and PROJECT_STATE.md's "Live lab validation" section for full
  results, including what remains unverified (the lab had zero enrolled
  endpoint clients).
- **Fixed a real bug found by this validation**:
  `internal/velociraptor/grpcclient.go`'s `GetClientInfo` previously
  treated Velociraptor's real `GetClient` response for an unknown client
  ID (a zero-value `ApiClient`, not an error) as a successful lookup,
  surfacing `velo_get_client_info` results with a hollow client object
  (`client_id: ""`) instead of an honest not-found message — a violation
  of this project's evidence-honesty principle. Added
  `ErrClientNotFound` (`internal/velociraptor/clients.go`) and a check
  for `resp.GetClientId() == ""` in `GetClientInfo`; the tool now
  reports `mode: "real"`, no `client` field, and an explanatory message
  instead. Covered by `TestGRPCClientGetClientInfoNotFound`.

### Added — v0.1.0 read-only Velociraptor visibility

- Implemented the four remaining read-only visibility tools:
  `velo_search_clients`, `velo_get_client_info`,
  `velo_list_artifact_names`, `velo_get_artifact_details`. The callable
  tool inventory is now exactly 8 (up from 4): all five visibility
  tools plus the three existing DFIR profile tools.
- `internal/velociraptor/veloapi/`: added `visibility.proto` (message
  definitions for `SearchClientsRequest`/`Response`, `ApiClient`,
  `AgentInformation`, `Uname`, `GetClientRequest`,
  `GetArtifactsRequest`, `ArtifactDescriptors`, `Artifact`,
  `ArtifactParameter`) and `api.proto` (the `service API` definition,
  moved out of `health.proto`, now declaring `Check`, `ListClients`,
  `GetClient`, and `GetArtifacts`). Field names/numbers copied from
  upstream Velociraptor's `api/proto/clients.proto`,
  `api/proto/artifacts.proto`, and `artifacts/proto/artifact.proto`
  (fetched directly from `Velocidex/velociraptor` on GitHub on
  2026-07-05). `Artifact` and its `ArtifactParameter` deliberately omit
  every field carrying VQL text (`sources`, `precondition`, etc.) — see
  docs/security-model.md's updated "Dependency surface" section.
  Regenerated with `buf generate` (same `protoc-gen-go`/`protoc-gen-go-grpc`
  toolchain as `health.proto`).
- Chose Velociraptor's purpose-built `ListClients`, `GetClient`, and
  `GetArtifacts` gRPC RPCs over the generic `Query` (streaming VQL) RPC
  for all four new tools: every caller-supplied value (search query,
  client ID, artifact name) is bound as a plain protobuf field, never a
  VQL string or parameter, so none of `internal/vql`'s
  template/parameter-binding machinery was needed for this milestone.
- `internal/velociraptor/grpcclient.go`: `grpcClient` gained
  `SearchClients`, `GetClientInfo`, `ListArtifactNames`, and
  `GetArtifactDetails`, each timeout-bounded via the existing
  `c.timeout` and routed through the existing `sanitizeTLSError`.
  `NewGRPCClient` gained a `maxRows` parameter
  (`config.VelociraptorConfig.MaxRows`) used to bound
  `SearchClients`/`ListArtifactNames` result counts server- and
  client-side (`boundLimit`/`effectiveMaxRows`; a non-positive value
  falls back to an internal `defaultMaxRows = 100` rather than being
  unbounded). Three new narrow seam interfaces (`clientSearcher`,
  `clientGetter`, `artifactCatalog`), mirroring the existing
  `healthChecker` pattern, keep every new method testable against fakes
  with no real TLS/network setup.
- `internal/validation`: added `SearchQuery` (length cap, rejects
  control characters) for `velo_search_clients`'s free-text filter —
  defense in depth only, since the query is always bound as a plain
  protobuf field, never concatenated into anything.
- `internal/policy`: added `Engine.AllowListAllArtifacts()`, backing
  `velo_list_artifact_names`'/`velo_get_artifact_details`' allowlist
  gating (`policy.allow_list_all_artifacts`); `ArtifactAllowed` remains
  the sole gate for actually *using* an artifact in a future
  collection/hunt.
- `internal/mcpserver/tools_visibility.go`: all four new handlers follow
  `velo_health_check`'s existing evidence-honesty pattern — every
  response carries a `mode` field (`"mock"`/`"real"`), connectivity or
  lookup failures are reported as a normal structured result (empty/`nil`
  data plus a `message`) rather than a Go-level tool error, and only
  input-validation failures (malformed client ID/artifact name/search
  query) or policy-allowlist blocks return a Go-level error with a
  `blocked` audit outcome. Every call still produces exactly one audit
  event.
- `cmd/agentic-velociraptor-mcp`: `buildDeps` now passes
  `cfg.Velociraptor.MaxRows` into `NewGRPCClient`.
- Tests: `grpcclient_test.go` gained fakes (`fakeClientSearcher`,
  `fakeClientGetter`, `fakeArtifactCatalog`) and success/error/timeout/
  limit-bounding/no-secret-leakage tests for all four new methods.
  `tools_visibility_test.go` gained mock-mode, real-mode
  success/error, invalid-input, and allowlist-gating tests for all four
  new handlers, plus a `fakeVisibilityClient`. `server_test.go`'s
  exact-inventory test now expects 8 tools, and gained
  `TestNewNeverRegistersUnsafeTools` (no collect/hunt/download/cancel/
  vql-named tool is ever callable) and an MCP-session-level call test
  for all four new tools. `internal/validation` gained `TestSearchQuery`.
  De-brittled two pre-existing, out-of-scope test assertions
  (`dfir.TestLoadDirParsesShippedProfiles`,
  `mcpserver.TestListDFIRProfilesHandlerReturnsShippedProfiles`) that
  hardcoded an exact profile count against `profiles/`'s contents, which
  had already grown independently of this milestone; both now assert
  "at least" the known profiles load, per this milestone's constraint
  not to touch `profiles/` or `docs/dfir-profiles.md`.
- Docs: README, PROJECT_STATE, docs/tool-reference.md,
  docs/security-model.md (extended "Dependency surface" and "Evidence
  honesty" sections), docs/lab-validation-plan.md (Phase 2 filled in),
  docs/configuration.md (`read_api_config_path`/`max_rows` field notes
  updated for the new tools).

### Added — v0.1.0-alpha.2 real Velociraptor health check

- Added `google.golang.org/grpc` and `google.golang.org/protobuf` as
  dependencies.
- New `internal/velociraptor/veloapi/` package: a minimal, hand-authored
  `.proto` (`health.proto`) mirroring only the single gRPC method this
  project calls on a real Velociraptor server — `API.Check`, modeled on
  the standard gRPC health-checking protocol — with field
  names/numbers/service names copied from upstream's own
  `api/proto/health.proto` and `api/proto/api.proto` for wire
  compatibility. Compiled to ordinary generated Go code with `buf` +
  `protoc-gen-go`/`protoc-gen-go-grpc`. Deliberately does **not** import
  the upstream `Velocidex/velociraptor` server module; see
  docs/security-model.md's new "Dependency surface" section.
- `internal/velociraptor/apiconfig.go`: `LoadAPIConfig` reads a real
  Velociraptor `api.config.yaml` (`ca_certificate`, `client_cert`,
  `client_private_key`, `api_connection_string`, `name`,
  `pinned_server_name`), fails closed on a missing file, a non-regular
  file, an overly permissive file mode (must be owner-only on POSIX),
  or missing required fields. `APIConfig.String()`/`GoString()` are
  hard-coded to a redacted placeholder so the type can never leak key
  material through an accidental format/log call.
- `internal/velociraptor/grpcclient.go`: `NewGRPCClient` builds an
  mTLS-authenticated gRPC connection (`tls.X509KeyPair` + CA pool +
  `credentials.NewTLS`, server name pinned to `pinned_server_name` or
  the upstream default `"VelociraptorServer"`) and a `grpcClient` that
  implements `HealthCheck` for real via the `Check` RPC, enforcing
  `timeout_seconds` via `context.WithTimeout`. Every other `Client`
  method remains the fail-closed placeholder (embedded, unchanged). A
  `healthChecker` seam interface lets tests exercise timeout/error
  handling with a fake, without any real TLS/network setup.
- `internal/mcpserver`: `Deps` gained `VelociraptorReadMode`
  (`"mock"`/`"real"`). `velo_health_check`'s handler now calls
  `ReadClient.HealthCheck` in real mode, reporting connectivity
  failures as a normal structured result (`status: "error"`,
  `velociraptor_connected: false`, safe message) rather than a
  Go-level tool error — Velociraptor being unreachable is data, not a
  tool failure. `HealthCheckOutput` gained `server_version` (always
  empty in this milestone: the `Check` RPC carries no version field).
- `internal/config`: `read_api_config_path` is now optional — empty
  means mock mode. `config.Validate` no longer requires it; the
  file-loadability check happens once, eagerly, in
  `cmd/agentic-velociraptor-mcp`'s `buildDeps`.
- `cmd/agentic-velociraptor-mcp`: `buildDeps` constructs a real
  `velociraptor.NewGRPCClient` when `read_api_config_path` is set
  (failing server startup outright, exit 1, if that config is
  missing/invalid/unsafe — never silently falling back to mock), or the
  mock placeholder when it's empty. `write_api_config_path` remains
  untouched by every code path. Added a small `resolveProfilesDir`
  helper: the default `--profiles-dir` now also tries a path relative to
  the running executable if the cwd-relative lookup fails, so the
  built binary depends less on being invoked from the repo root; an
  explicit `--profiles-dir` is always honored as given.
- Tests: `internal/velociraptor` gained `apiconfig_test.go` (missing
  file, empty path, directory, overly permissive mode, missing fields,
  valid parse, no secret leakage from `String`/`GoString`) and
  `grpcclient_test.go` (success, `NOT_SERVING`, transport error,
  timeout enforcement, no secret leakage via `sanitizeTLSError`, all
  against a fake `healthChecker`). `internal/mcpserver` gained real-mode
  success/error tests for `velo_health_check` (including a check that a
  connectivity failure produces a normal result, not `IsError`/a Go
  error, and that no PEM content reaches the output or audit log).
  `cmd/agentic-velociraptor-mcp` gained tests for a broken configured
  `read_api_config_path` (fails closed), mock-mode `buildDeps`, and
  `resolveProfilesDir`'s three branches. Manually smoke-tested mock mode
  end to end over a real stdio subprocess (see docs/lab-validation-plan.md
  Phase 1).
- `go.mod`'s `go` directive requires Go 1.25+ (unchanged from
  v0.1.0-alpha.1, now load-bearing for `google.golang.org/grpc`/`protobuf`
  too).
- Docs: README, PROJECT_PLAN, PROJECT_STATE, docs/configuration.md (new
  "The read API config file" section), docs/security-model.md (updated
  secrets-handling and evidence-honesty sections, new "Dependency
  surface" section), docs/lab-validation-plan.md (Phase 1 split into
  done-via-fakes vs. needs-a-real-lab-server), docs/velociraptor-permissions.md
  (Check RPC needs no ACL beyond a valid cert; `pinned_server_name`
  note), examples/client-configs (mock-mode-by-default example config,
  new `reader.api.config.example.yaml`), examples/inspector/README.md.

### Added — v0.1.0-alpha.1 MCP skeleton

- Added `github.com/modelcontextprotocol/go-sdk` v1.6.1 as a dependency
  (Go module `go` directive auto-bumped 1.23.4 → 1.25.0 by the SDK's
  minimum Go version requirement).
- `internal/mcpserver.Server` now wraps a real `*mcp.Server` and serves
  the stdio transport (`mcp.StdioTransport`) via `Server.Run(ctx)`,
  replacing the v0.0.x placeholder that panicked.
- Registered exactly 4 callable MCP tools (of the 24 planned):
  `velo_health_check` (static mock — no Velociraptor call yet),
  `velo_list_dfir_profiles`, `velo_get_dfir_profile`,
  `velo_validate_dfir_profile`. All four are read-only,
  non-destructive, closed-world per their `ToolAnnotations`, and audit
  every call. The remaining 20 tools stay unregistered `ToolSpec`
  metadata — unimplemented tools are never made callable.
- `cmd/agentic-velociraptor-mcp`: the default command (given `--config`)
  now loads and validates config, builds the DFIR profile registry, the
  audit sink, and the policy engine, and runs the MCP server over stdio
  until the client disconnects or SIGINT/SIGTERM. Added `--profiles-dir`
  flag. A missing/invalid config file fails closed (exit 1) without ever
  starting the transport.
- Tests: `internal/mcpserver` gained an in-memory-transport test
  asserting the exact 4-tool inventory and read-only annotations, plus
  unit tests for each handler (health check mock output; profile
  list/get/validate including not-found and invalid-name-syntax safe
  error paths; audit outcome assertions via a fake sink).
  `cmd/agentic-velociraptor-mcp` gained a test for the fail-closed
  missing-config-file path. Manually verified once end-to-end over a
  real stdio subprocess via the SDK's `CommandTransport` (see
  docs/lab-validation-plan.md Phase 0).
- Docs: README, PROJECT_PLAN (MCP Security Best-Practice Integration
  section), docs/tool-reference.md, docs/security-model.md (new
  "MCP-specific security practices" section: no credential passthrough,
  no arbitrary URL fetching, unimplemented tools never registered,
  confused-deputy mitigation via approval fingerprinting, tool/scope
  minimization), docs/lab-validation-plan.md (new Phase 0).

### Added — v0.0.x project foundation

- Repository skeleton: `cmd/`, `internal/{audit,approval,config,dfir,
  mcpserver,policy,validation,velociraptor,vql}`, `profiles/`, `docs/`,
  `examples/`, `tests/`.
- Go module `github.com/hdyrawan/agentic-velociraptor-mcp` (Go 1.23).
- CLI entrypoint with `--version` and `--help`; no MCP server behavior
  yet.
- `internal/config`: full config struct tree, YAML loader, structural
  validator, conservative `Default()`.
- `internal/audit`: audit event model with exhaustive
  success/blocked/error outcomes, JSONL sink, redaction placeholder.
- `internal/approval`: approval request/decision model, store interface,
  fingerprinting helper. No approval mechanism implemented yet.
- `internal/policy`: policy engine over config, allow/require-approval/
  deny decision model, dangerous-permissions checklist.
- `internal/dfir`: DFIR profile model, YAML-backed registry, artifact
  allowlist cross-check validation.
- `internal/validation`: client ID, artifact name, DFIR profile name,
  hash/IP/domain, and hunt scope validators.
- `internal/velociraptor`: `Client` interface (health/clients/artifacts/
  flows/hunts/uploads) with a fail-closed placeholder implementation; no
  real gRPC connection yet.
- `internal/vql`: allowlisted IOC-hunt template constants and a
  fail-closed `Bind`; no VQL string construction or execution.
- `internal/mcpserver`: tool metadata (`ToolSpec`) for all 24 planned
  stable-core tools, grouped by concern; server/deps shape defined,
  `Run` intentionally panics until v0.1.0-alpha.1.
- 3 of 15 planned DFIR profile definitions
  (`windows_basic_triage`, `windows_ransomware_triage`,
  `linux_basic_triage`).
- Documentation: architecture, security model, approval flow,
  configuration, tool reference, DFIR profiles, Velociraptor
  permissions, lab validation plan, production deployment.
- Apache-2.0 LICENSE, README, PROJECT_PLAN, PROJECT_STATE.
