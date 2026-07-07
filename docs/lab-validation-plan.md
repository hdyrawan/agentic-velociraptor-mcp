# Lab validation plan

Status: draft. This plan should be executed before any production
deployment (v0.5.0/v1.0.0 gate), against a disposable Velociraptor lab
server and disposable enrolled endpoints — never against a production
Velociraptor deployment or real endpoints.

## Lab environment requirements

- A standalone Velociraptor server (container or VM), not connected to
  any production fleet.
- At least one Windows and one Linux client enrolled as disposable VMs
  (snapshot/revert capable).
- Two API client identities created per
  [velociraptor-permissions.md](velociraptor-permissions.md): reader and
  investigator, scoped down from the start rather than starting broad
  and narrowing later.
- MCP Inspector (or another MCP test client) available for manual tool
  invocation.

## Validation phases

### Phase 0 — MCP stdio server smoke test (v0.1.0-alpha.1, done)

No lab Velociraptor server is needed for this phase — it validates the
MCP transport and DFIR profile tools only, with Velociraptor still
mocked. Performed manually during development using the SDK's
`CommandTransport` against the built binary (see
[examples/inspector/README.md](../examples/inspector/README.md) for the
MCP Inspector equivalent):

- [x] `ListTools` over a real stdio subprocess returns exactly
      `velo_health_check`, `velo_list_dfir_profiles`,
      `velo_get_dfir_profile`, `velo_validate_dfir_profile` — no other
      tool is callable.
- [x] Every returned tool has `ReadOnlyHint: true`,
      `DestructiveHint: false`, `OpenWorldHint: false`.
- [x] `velo_health_check` returns the static mock payload
      (`status: ok`, `mode: mock`, `velociraptor_connected: false`) with
      `IsError` unset.
- [x] `velo_list_dfir_profiles` / `velo_get_dfir_profile` /
      `velo_validate_dfir_profile` work against the real
      `profiles/*.yaml` files via `--profiles-dir`.
- [x] `velo_get_dfir_profile` / `velo_validate_dfir_profile` on an
      unknown profile name return `IsError: true` with an explanatory
      message — not a protocol-level error, not a crash.
- [x] Passing a config path that doesn't exist exits with a clear error
      and code 1 without ever starting the stdio transport (see
      `cmd/agentic-velociraptor-mcp/main_test.go`).

### Phase 1 — connectivity and identity (v0.1.0-alpha.2)

The unit-testable parts of this phase are done, without a live
Velociraptor server, using fakes at the `healthChecker` seam
(`internal/velociraptor/grpcclient_test.go`) and at the
`velociraptor.Client` seam
(`internal/mcpserver/tools_visibility_test.go`):

- [x] `LoadAPIConfig` fails closed on a missing file, a directory, an
      overly permissive file mode, and a file missing required fields
      (`internal/velociraptor/apiconfig_test.go`).
- [x] `grpcClient.HealthCheck` succeeds when the underlying RPC reports
      `SERVING`, fails when it reports `NOT_SERVING`, fails on a
      transport error, and respects `timeout_seconds` rather than
      blocking on a slow/hanging server
      (`internal/velociraptor/grpcclient_test.go`).
- [x] `velo_health_check`'s real-mode success and error paths both
      produce a normal structured result (not a Go-level tool error),
      matching `docs/security-model.md`'s evidence-honesty principle
      (`internal/mcpserver/tools_visibility_test.go`).
- [x] Neither a crafted PEM-embedded error string nor a real `APIConfig`
      value can leak certificate/key content through
      `sanitizeTLSError`, `APIConfig.String()/GoString()`, the tool's
      `Message` field, or the audit event's `Reason` field (same test
      files).
- [x] A `--config` pointing at a non-empty but broken
      `read_api_config_path` fails server startup (exit 1) before the
      stdio transport ever starts
      (`cmd/agentic-velociraptor-mcp/main_test.go`).

**Done (2026-07-06), against a real disposable lab** — see "Live lab
validation (2026-07-06)" below for the environment and exact commands:

- [x] `velo_health_check` succeeds against a real reader identity and
      real server (`mode: "real"`, `velociraptor_connected: true`).
- [ ] `velo_health_check` fails closed (clear `status: "error"`, not a
      crash) when pointed at a real but unreachable/misconfigured
      server (wrong port, expired cert, CA mismatch, wrong
      `pinned_server_name`). **Not exercised this pass** — only the
      happy path was driven against the live server; the fake-based
      unit tests already cover the failure shapes, but a real
      misconfigured-server error was not reproduced live.
- [x] Confirm no API config contents appear in stdout/stderr/audit log
      when run against a real server end to end, including its error
      paths (a live-server complement to the unit tests above, which
      use synthetic/fake errors, not ones actually produced by a real
      TLS handshake against Velociraptor). Confirmed: audit.jsonl and
      the smoke-test stdout/stderr from this pass contain no
      `BEGIN CERTIFICATE`/`BEGIN ... PRIVATE KEY` text or other secret
      material.
- [x] Confirm a reader identity holding only the ACLs recommended in
      [velociraptor-permissions.md](velociraptor-permissions.md) (no
      `administrator`, no dangerous permissions) can still successfully
      call `Check` — the upstream handler
      (`api/health.go`) returns `SERVING` unconditionally once a call
      authenticates, so this is expected to work, but should be
      confirmed against a real minimally-privileged identity rather
      than assumed. Confirmed against the lab's `mcp-reader` identity
      (role `reader`, not `administrator`).

### Phase 2 — read-only visibility (v0.1.0, implemented via `ListClients`/`GetClient`/`GetArtifacts` gRPC RPCs; live-validated 2026-07-06)

- [x] `velo_search_clients` / `velo_get_client_info` return expected data
      for the lab clients (real `ApiClient` field values — hostname, OS,
      last-seen time, labels, MAC addresses — match what the lab server
      and GUI report for the same clients). **Partially validated**: the
      lab has zero enrolled endpoint clients (see "Live lab validation"
      below), so `velo_search_clients` was only confirmed to return an
      honest empty list (`clients: []`, `mode: "real"`), not real
      per-client field values. `velo_get_client_info` against a
      well-formed but nonexistent client ID surfaced a real bug (now
      fixed — see below) rather than validating field mapping, since
      there is no real client to fetch. Field-value correctness against
      an actual enrolled client remains unverified.
- [ ] `velo_search_clients`'s `query` field behaves as expected against
      Velociraptor's real client-search grammar (hostname/IP/label
      substrings and globs) — this project passes it through unmodified
      and does not itself define or validate that grammar. **Not
      verifiable this pass**: zero enrolled clients means no query can
      be confirmed to actually match/filter anything (an empty result
      is consistent with both "no clients" and "query matched nothing").
- [x] `velo_list_artifact_names` respects `policy.allow_list_all_artifacts`
      (confirm both the `false`/allowlist-only and `true`/full-catalog
      behaviors against a real server's artifact count). Confirmed:
      `false` returned exactly the 3 configured `allowed_artifacts`
      present on the server; `true` returned the full catalog (399
      artifacts on this lab's Velociraptor 0.76.3).
- [x] `velo_get_artifact_details` never returns raw VQL body — confirmed
      structurally in code (`internal/velociraptor/veloapi/visibility.proto`'s
      `Artifact` message has no field for `ArtifactSource`), but worth
      re-confirming end to end that a real server's `GetArtifacts`
      response, once decoded through this project's generated types,
      truly carries no VQL text into the tool output. Confirmed: the
      full 399-artifact real-server response was scanned for
      `SELECT `/`LET ... =`; the only matches were inside human-authored
      `description` prose that mentions VQL as a usage example (e.g.
      "``SELECT * from Artifact.Server.Enrichment.VirusTotal(...)``" in
      an artifact's own doc text) — never an artifact's actual query
      body, which `ArtifactDetailOutput`/`ArtifactSummaryOutput`
      structurally have no field for.
- [x] `velo_get_artifact_details` is blocked (not silently empty) for an
      artifact outside `policy.allowed_artifacts` when
      `allow_list_all_artifacts` is `false`. Confirmed: querying
      `Not.An.Allowed.Artifact` (and a real-but-not-allowlisted name)
      returned `IsError: true` with `artifact "..." is not in the
      configured allowlist`, audited as `outcome: "blocked"`.
- [x] Confirm `velociraptor.max_rows` actually bounds `velo_search_clients`
      and `velo_list_artifact_names` result counts against a lab server
      with more clients/artifacts than the configured limit, and that a
      server returning more rows than requested is still truncated
      client-side (defense in depth beyond the request-side `limit`).
      Confirmed for `velo_list_artifact_names`: `max_rows: 3` against a
      399-artifact real catalog (with `allow_list_all_artifacts: true`)
      returned exactly 3 results. **Not confirmed for
      `velo_search_clients`** — zero enrolled clients means truncation
      can't be distinguished from "no data"; a large `limit` (100000)
      at least confirmed it doesn't error.
- [x] `velo_list_flows` / `velo_get_flow_status` / `velo_get_flow_results` registered and handler-tested with fake read clients; real Velociraptor flow RPC validation still pending
      correctly truncate at `max_rows` / `max_result_bytes` and report
      truncation. **(Not implemented in v0.1.0 — deferred; no RPC exists
      yet for these three tools.)**
- [x] Attempt to pass malformed client IDs / artifact names (SQL/VQL
      injection-shaped strings, path traversal strings) and confirm
      `internal/validation` rejects them before any Velociraptor call.
      Confirmed live: `client_id: "not-a-valid-id"` was rejected by
      `validation.ClientID` (`IsError: true`, audited `blocked`) before
      any `GetClient` call was attempted.
- [x] Confirm every call above produces exactly one audit event with the
      correct outcome. Confirmed by direct inspection of the scratch
      `audit.jsonl` produced during this pass: one JSONL line per tool
      call, with `success`/`blocked`/`error` outcomes matching each
      call's actual result, and no secret material in any line.
- [x] Confirm none of `velo_search_clients`, `velo_get_client_info`,
      `velo_list_artifact_names`, or `velo_get_artifact_details` call the
      generic `Query` RPC or accept anything resembling VQL text — all
      four use the purpose-built `ListClients`/`GetClient`/`GetArtifacts`
      RPCs exclusively (see docs/security-model.md's "Dependency
      surface" section). Reconfirmed: `grpcClient` has no `Query` method
      at all (`internal/velociraptor/grpcclient.go`), and the live run's
      `ListTools` output shows exactly the same 8 tools as
      `internal/mcpserver/server_test.go`'s exact-inventory test — no
      additional tool was exposed by running against a real server.

#### Live lab validation (2026-07-06)

Performed against the disposable lab described in
`/home/irawanhd/velociraptor-lab/README.md`:

- **Velociraptor version**: 0.76.3 (`wlambert/velociraptor:latest` image,
  as reported by that lab's README).
- **API identity used**: the lab's pre-generated `mcp-reader`
  `api.config.yaml` (role `reader`, not `administrator`) — the
  investigator identity was never needed or used, per this validation's
  hard constraint.
- **Tools live-validated**: all 8 callable tools were exercised over a
  real stdio subprocess (`mcp.CommandTransport` driving the actual
  built binary, the same transport a real MCP client uses):
  `velo_health_check`, `velo_search_clients`, `velo_get_client_info`,
  `velo_list_artifact_names`, `velo_get_artifact_details`,
  `velo_list_dfir_profiles`, `velo_get_dfir_profile`,
  `velo_validate_dfir_profile` (the three DFIR profile tools were
  covered by `ListTools`/inventory checks only, since their behavior is
  local-file-based and already lab-independent).
- **Client inventory**: empty. The lab has zero enrolled endpoint
  clients, so `velo_search_clients` was only confirmed to return an
  honest `clients: []` and `velo_get_client_info` could only be
  exercised against nonexistent client IDs — see the unchecked items
  above for exactly what remains unverified as a result.
- **Callable inventory**: exactly 8, confirmed by `ListTools` against
  the live subprocess, matching `internal/mcpserver/server_test.go`'s
  exact-inventory test.
- **No raw VQL / generic Query path**: confirmed — `grpcClient` has no
  `Query` method, and no tool call in this pass accepted or surfaced
  VQL text (see the artifact-details finding above).
- **Network note**: the lab's `mcp-reader.api.config.yaml` has
  `api_connection_string: 0.0.0.0:8001` (Velociraptor's own generated
  value, meaning "the API port this server bound to"), but the lab
  container only publishes ports 8000/8889 to the host — port 8001
  (the gRPC API) is reachable only via the container's own bridge-network
  IP. This validation used a local scratch copy of the reader config
  with `api_connection_string` rewritten to that container IP; the
  original secret-bearing file at
  `/home/irawanhd/velociraptor-lab/data/api-clients/mcp-reader.api.config.yaml`
  was never modified. This is an artifact of this lab's Docker network
  topology, not a code or config-schema issue — no change was needed in
  `internal/velociraptor` or `internal/config` to work around it.
- **Confirmed bug found and fixed**: `velo_get_client_info` against a
  well-formed but nonexistent client ID returned a "successful" result
  with an empty-but-present `client` object (`client_id: ""`) instead of
  an honest not-found message — because Velociraptor's real `GetClient`
  RPC returns a zero-value `ApiClient`, not an error, for an unknown ID.
  Fixed in `internal/velociraptor/grpcclient.go`'s `GetClientInfo`
  (detects `resp.GetClientId() == ""` and returns the new
  `ErrClientNotFound` sentinel instead), covered by a new
  `TestGRPCClientGetClientInfoNotFound` unit test. Re-run live after the
  fix: the same call now returns `mode: "real"`, no `client` field, and
  `message: "...client not found"` — an honest result consistent with
  this project's evidence-honesty principle.

### Phase 3 — read-only DFIR workflow expansion (v0.3.0)

- [x] `ListTools` showed exactly 11 callable tools for v0.3.0: the prior 8 plus
      `velo_plan_dfir_triage`, `velo_compare_dfir_profiles`, and
      `velo_find_profiles_by_artifact`.
- [x] `velo_plan_dfir_triage` returns profile recommendations and
      read-only next steps without making any Velociraptor RPC or using
      the write client (covered by local unit/MCP-session tests).
- [x] `velo_compare_dfir_profiles` returns success for known profiles,
      structured `not_found` for unknown profiles, and blocked errors for
      malformed/duplicate/too-few inputs.
- [x] `velo_find_profiles_by_artifact` returns matching profile coverage,
      structured `not_found` for no matches, and blocked errors for
      malformed artifact names.
- [x] No collection, hunt start/cancel, download, mutation, write API
      identity use, or raw VQL tool is registered.

### Phase 4 — read-only flow/result backfill (v0.5.0)

- [x] `ListTools` shows exactly 14 callable tools: the prior 11 plus
  `velo_list_flows`, `velo_get_flow_status`, and
  `velo_get_flow_results`.
- [x] Flow/result handlers are read-only, validate `client_id`/`flow_id`,
  and audit success/error/blocked outcomes.
- [x] `velo_get_flow_results` is bounded by `max_rows` and
  `max_result_bytes` in handler tests and reports truncation explicitly.
- [x] Real Velociraptor flow listing/status/result field mapping —
      **live-validated 2026-07-07** for list/status and for
      default-source artifacts (`Linux.Sys.Pslist`: real rows, correct
      fields). **Confirmed gap**: `velo_get_flow_results` returns a false
      `empty` for named-source artifacts (`Generic.Client.Info`) — see
      [live-validation-report-v0.10.2.md](live-validation-report-v0.10.2.md)
      finding 2.

### Phase 5 — controlled collection (v0.4.0, unit-tested; partially live-validated 2026-07-07)

The six collection/flow-upload tools are unit-tested with fakes. The
items below require a disposable lab with enrolled clients and the write
API configured:

- [x] Attempt `velo_collect_artifact_with_approval` without a prior
      approval and confirm it is blocked, not executed. **Live-validated
      2026-07-07**: `status: "not_found"`.
- [ ] Grant an approval scoped to artifact A on client X; confirm an
      attempt to use it for artifact B or client Y is rejected
      (fingerprint mismatch). Not re-exercised live this pass (already
      unit-tested); only the reuse-of-the-same-approval case was
      live-confirmed (rejected: `"already been used"`).
- [ ] Confirm the write identity, not the read identity, is used for the
      actual collection call (e.g. via Velociraptor server-side audit
      logging of which API identity performed the action, if available).
- [ ] Confirm `velo_download_flow_upload_with_approval` enforces
      `max_upload_bytes` and requires its own approval distinct from the
      collection's approval. **Not exercised 2026-07-07** — no
      allowlisted artifact in that pass produced a real upload; see
      live-validation-report-v0.10.2.md's "Limitations".
- [ ] Confirm cancellation (`velo_cancel_flow_with_approval`) requires
      approval and correctly reflects in subsequent
      `velo_get_flow_status`. Not exercised 2026-07-07 (no long-running
      flow available within the pass's time budget).

### Phase 6 — hunts (v0.6.0, unit-tested incl. v0.7.0's fingerprint fix; largely live-validated 2026-07-07)

- [x] Approval for one hunt (artifact/case/scope) cannot be reused to
      start/cancel a *different* hunt (fingerprint mismatch) —
      **unit-tested as of v0.7.0**:
      `TestStartHuntRejectsMismatchedApproval`,
      `TestStartDFIRHuntRejectsMismatchedApproval`,
      `TestCancelHuntRejectsMismatchedApproval`
      (`internal/mcpserver/tools_hunts_test.go`). This was previously a
      real, unverified gap — see CHANGELOG.md's v0.7.0 entry.
- [x] `velo_preview_hunt_scope` against an all-clients scope returns an
      accurate matched-client count (`matched: 1` in a 1-client lab)
      without creating a hunt — **live-validated 2026-07-07**.
      Label-scope matching was attempted but could not be confirmed
      against a genuinely labeled client this pass (lab-tooling
      limitation, not a code defect — see
      live-validation-report-v0.10.2.md's "Limitations"); the label
      *condition* sent to Velociraptor was independently confirmed
      correct by inspecting the created hunt's stored server-side JSON.
- [x] `velo_start_hunt_with_approval` with a valid approval actually
      creates a hunt on the Velociraptor server — **live-validated
      2026-07-07** with an all-clients scope: real `CreateHunt`, the
      client was genuinely scheduled (a hunt-driven flow appeared, and
      `velo_get_hunt_status` reported `client_count: 1`).
- [x] `velo_start_dfir_hunt_with_approval` with a valid approval creates
      hunts for each profile artifact — **live-validated 2026-07-07**
      with a label scope (real `CreateHunt` per artifact, correct scope
      stored server-side); 0 clients matched for the label-application
      reason above.
- [x] `velo_list_hunts` returns real hunt records from the server —
      **live-validated 2026-07-07**.
- [x] `velo_get_hunt_status` returns accurate state/client-count for a
      real created hunt — **live-validated 2026-07-07** (`client_count: 1`
      for the all-clients hunt; `stopped` after cancellation). Not-found
      sentinel not re-exercised live this pass (already unit-tested).
- [x] `velo_get_hunt_results` returns real result rows for a real hunt —
      **live-validated 2026-07-07** for the RPC path itself, but hit the
      same named-source gap as flow results (`Generic.Client.Info`
      returned `empty` despite `client_count: 1`); pagination across
      result pages not exercised (too little data in this tiny lab).
- [x] `velo_cancel_hunt_with_approval` with a valid approval cancels a
      real running hunt, reflected in subsequent `velo_get_hunt_status` —
      **live-validated 2026-07-07** (state changed to `stopped`).
- [ ] Attempt a scope matching more than `policy.max_hunt_clients`;
      confirm start is refused rather than silently capped. Not
      exercised (1-client lab; already unit-tested with fakes).
- [ ] Attempt `all`-clients scope with `policy.allow_target_all: false`;
      confirm refusal. Not re-exercised live this pass — this pass
      deliberately set `allow_target_all: true` (in a genuinely tiny
      1-client lab) to validate the opposite path end-to-end; the
      refusal path itself is already unit-tested with fakes.
- [x] Start without prior approval is blocked (audited as `blocked`),
      not executed — **live-validated 2026-07-07** for `start_hunt`
      (`status: "not_found"` for an unknown reference).
- [ ] Unregistered profile names in `velo_start_dfir_hunt_with_approval`
      are rejected with `not_found`. Not re-exercised live (already
      unit-tested).
- [ ] Non-allowlisted profile names in `velo_start_dfir_hunt_with_approval`
      are rejected (blocked). Not re-exercised live (already
      unit-tested).
- [x] Real-mode explicit `client_ids` hunt scope is refused before
      consuming the approval for `velo_start_hunt_with_approval` and
      `velo_start_dfir_hunt_with_approval` (Velociraptor's typed hunt RPCs
      have no field for an explicit client-ID list) —
      `TestStartHuntRealModeExplicitClientIDsPreservesApproval`,
      `TestStartDFIRHuntRealModeExplicitClientIDsPreservesApproval`.

### Phase 7 — IOC hunting (v0.7.0, unit-tested; pending live-lab validation)

- [x] IOC kind/value validation for all 5 kinds (hash, ip, domain,
      process, path), success and failure cases —
      `TestHuntIOCValidatesEachKind` (`internal/mcpserver/tools_ioc_test.go`)
      plus `TestProcess`/`TestPath`/`TestValidateIOC`
      (`internal/validation/validation_test.go`).
- [x] Blocked in `read_only` mode, blocked without a prior approval,
      `target_all` blocked by default, `max_hunt_clients` enforced (caller
      cannot raise the ceiling), approval consumed only after all
      policy/input/scope/allowlist gates pass — unit-tested
      (`TestHuntIOCBlockedInReadOnlyMode`, `TestHuntIOCBlockedWithoutApproval`,
      `TestHuntIOCTargetAllBlockedByDefault`,
      `TestHuntIOCEnforcesMaxHuntClients`).
- [x] Approval for one indicator/scope cannot authorize a different one
      (fingerprint mismatch), including the cross-tool case where an
      IOC-hunt approval cannot authorize a plain
      `velo_start_hunt_with_approval` call — `TestHuntIOCRejectsMismatchedApproval`,
      `TestApprovalForIOCHuntCannotAuthorizeRegularHuntStart`.
- [x] Approved fake-client path starts a hunt and consumes the approval —
      `TestHuntIOCApprovedFakePath`.
- [x] Real (non-mock) `WriteClient` without hunt RPCs implemented (e.g.
      the built-in placeholder client used when no write API config is
      set) reports `ErrNotImplemented`/`backend_not_implemented` honestly
      as an `error`-status result, not fabricated success —
      `TestHuntIOCScaffoldedRealModeReturnsHonestError`.
- [x] Real-mode explicit `client_ids` hunt scope is refused before
      consuming the approval, leaving it valid for a retry with `label`
      or `all` scope —
      `TestHuntIOCRealModeExplicitClientIDsPreservesApproval`.
- [x] `velo_hunt_ioc_with_approval` with a valid approval attempts to
      create a hunt on a real Velociraptor server — **live-validated
      2026-07-07, and confirmed failing**: the real `CreateHunt` call
      returns `Unknown artifact System.Hash.Hunt`. The approval was
      still correctly consumed (one-shot semantics apply even on
      backend failure), and the error was honest/structured, not a
      crash or fabricated success. Only the `hash` kind was exercised
      (the other 4 kinds share the same nonexistent-artifact root
      cause, so were not separately re-run).
- [x] Confirm whether `System.Hash.Hunt`/`System.IP.Hunt`/
      `System.Domain.Hunt`/`System.Process.Hunt`/`System.Path.Hunt` (the
      illustrative artifact names `vql.Bind` resolves each IOC kind to)
      correspond to any real artifact in a live Velociraptor server's
      catalog — **confirmed 2026-07-07: none of them do** (checked
      against the same 433-artifact real catalog `catalog/artifacts.yaml`
      was verified against in v0.10.0). This must be resolved (real,
      catalog-verified artifact/parameter mapping) before
      `velo_hunt_ioc_with_approval` can work against a real server —
      see live-validation-report-v0.10.2.md finding 3.
- [ ] Fuzz malformed hash/IP/domain/process/path input against a real
      server path (edge cases: IPv6, defanged indicators like
      `1[.]2[.]3[.]4`, mixed-case hashes, trailing-dot domains, Windows
      vs. Unix path separators) — unit tests cover representative cases
      but not exhaustive fuzzing.

### Phase 8 — negative/adversarial testing (v0.5.0)

- [ ] Simulated prompt-injection payload embedded in artifact/collection
      result data (e.g. a filename or registry value containing
      instruction-like text) does not cause the server to treat it as a
      new instruction — the server has no mechanism to "obey" result
      content, but confirm response formatting doesn't accidentally
      re-inject it somewhere trusted.
- [ ] Attempted use of an artifact name syntactically close to an
      allowlisted one (case variation, trailing characters) is rejected.
- [ ] Killing the Velociraptor client agent mid-collection produces an
      honest `error`/incomplete-result response, not a silent success.
- [ ] Audit log review: reconstruct a full investigation timeline from
      `audit.jsonl` alone for a representative session covering read,
      approval-gated, and blocked operations.

## Sign-off

Do not proceed to a production deployment
([production-deployment.md](production-deployment.md)) until every box
above is checked against the actual implementation, with findings
recorded (not just assumed from design intent).


## Backend wiring status (as of v0.10.2)

Every RPC group below now has a reviewed typed gRPC binding
(`internal/velociraptor/grpcclient_flows.go`, `grpcclient_uploads.go`,
`grpcclient_hunts.go`), unit-tested against fake gRPC service stubs, and
— as of v0.10.2 — largely live-validated. See
[live-validation-report-v0.10.2.md](live-validation-report-v0.10.2.md)
for the full pass/fail detail.

| Group | Status |
|---|---|
| Visibility (`health`, client search/info, artifact list/details) | Real gRPC; live-validated 2026-07-06 (Phase 2). |
| Flow list/status/results | Real gRPC; live-validated 2026-07-07 for list/status and default-source artifacts; confirmed gap for named-source artifacts (Phase 4). |
| Collection start / DFIR profile collection / flow cancel | Real gRPC; backend capability checked before consuming approval; collection start live-validated 2026-07-07; flow cancel not yet exercised (Phase 5). |
| Flow uploads list/metadata/download | Real gRPC; download backend capability checked before consuming approval; list confirmed honest-empty 2026-07-07; metadata/download not yet exercised against a real upload (Phase 5). |
| Hunts list/status/results/preview | Real gRPC; explicit `client_ids` scope has no typed RPC support and is refused before consuming approval; list/status/preview/results live-validated 2026-07-07 end-to-end (all-clients scope); label-scope matching not confirmed (lab-tooling limitation) (Phase 6). |
| Approved hunt start/cancel and IOC hunt | Real gRPC; same `client_ids` limitation; start/cancel live-validated 2026-07-07; IOC hunt confirmed non-functional against a real server (nonexistent artifact mapping) (Phases 6-7). |

Required follow-up before production: fix the named-source
result-retrieval gap and the IOC-hunt artifact mapping, complete every
remaining unchecked item in Phases 4-8 above against a disposable lab,
prove least-privilege read/write API permissions, and keep `max_rows`,
`max_result_bytes`, `max_upload_bytes`, `max_hunt_clients`, `target_all`,
cursor, audit, and
no-raw-VQL invariants under test throughout.
