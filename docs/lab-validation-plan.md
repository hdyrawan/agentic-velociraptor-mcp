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
- [ ] `velo_list_flows` / `velo_get_flow_status` / `velo_get_flow_results`
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

- [x] `ListTools` shows exactly 11 callable tools: the prior 8 plus
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

### Phase 4 — controlled collection (deferred; not v0.3.0)

- [ ] Attempt `velo_collect_artifact_with_approval` without a prior
      approval and confirm it is blocked, not executed.
- [ ] Grant an approval scoped to artifact A on client X; confirm an
      attempt to use it for artifact B or client Y is rejected
      (fingerprint mismatch).
- [ ] Confirm the write identity, not the read identity, is used for the
      actual collection call (e.g. via Velociraptor server-side audit
      logging of which API identity performed the action, if available).
- [ ] Confirm `velo_download_flow_upload_with_approval` enforces
      `max_upload_bytes` and requires its own approval distinct from the
      collection's approval.
- [ ] Confirm cancellation (`velo_cancel_flow_with_approval`) requires
      approval and correctly reflects in subsequent
      `velo_get_flow_status`.

### Phase 5 — hunts (deferred; not v0.3.0)

- [ ] `velo_preview_hunt_scope` against a label/explicit-client-list
      scope returns an accurate matched-client count without creating a
      hunt.
- [ ] Attempt a scope matching more than `policy.max_hunt_clients`;
      confirm start is refused (or requires explicit override per final
      design) rather than silently capped.
- [ ] Attempt `all`-clients scope with `policy.allow_target_all: false`;
      confirm refusal.
- [ ] `velo_cancel_hunt_with_approval` requires approval and is reflected
      in `velo_get_hunt_status`.

### Phase 6 — DFIR profiles and IOC hunting (future)

- [ ] `velo_validate_dfir_profile` correctly flags a profile referencing
      a non-allowlisted artifact.
- [ ] `velo_collect_dfir_profile_with_approval` /
      `velo_start_dfir_hunt_with_approval` expand to exactly the
      profile's artifact list, no more.
- [ ] `velo_hunt_ioc_with_approval` rejects malformed
      hash/IP/domain input (fuzz with edge cases: IPv6, defanged
      indicators like `1[.]2[.]3[.]4`, mixed-case hashes, trailing dot
      domains).

### Phase 7 — negative/adversarial testing (v0.5.0)

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
