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

**Not yet done — requires a real disposable Velociraptor lab server**
(see "Lab environment requirements" above); none of the following can be
verified by unit tests alone:

- [ ] `velo_health_check` succeeds against a real reader identity and
      real server (`mode: "real"`, `velociraptor_connected: true`).
- [ ] `velo_health_check` fails closed (clear `status: "error"`, not a
      crash) when pointed at a real but unreachable/misconfigured
      server (wrong port, expired cert, CA mismatch, wrong
      `pinned_server_name`).
- [ ] Confirm no API config contents appear in stdout/stderr/audit log
      when run against a real server end to end, including its error
      paths (a live-server complement to the unit tests above, which
      use synthetic/fake errors, not ones actually produced by a real
      TLS handshake against Velociraptor).
- [ ] Confirm a reader identity holding only the ACLs recommended in
      [velociraptor-permissions.md](velociraptor-permissions.md) (no
      `administrator`, no dangerous permissions) can still successfully
      call `Check` — the upstream handler
      (`api/health.go`) returns `SERVING` unconditionally once a call
      authenticates, so this is expected to work, but should be
      confirmed against a real minimally-privileged identity rather
      than assumed.

### Phase 2 — read-only visibility (v0.1.0)

- [ ] `velo_search_clients` / `velo_get_client_info` return expected data
      for the lab clients.
- [ ] `velo_list_artifact_names` respects `allow_list_all_artifacts`.
- [ ] `velo_get_artifact_details` never returns raw VQL body (confirm
      against current implementation decision).
- [ ] `velo_list_flows` / `velo_get_flow_status` / `velo_get_flow_results`
      correctly truncate at `max_rows` / `max_result_bytes` and report
      truncation.
- [ ] Attempt to pass malformed client IDs / artifact names (SQL/VQL
      injection-shaped strings, path traversal strings) and confirm
      `internal/validation` rejects them before any Velociraptor call.
- [ ] Confirm every call above produces exactly one audit event with the
      correct outcome.

### Phase 3 — controlled collection (v0.2.0)

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

### Phase 4 — hunts (v0.3.0)

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

### Phase 5 — DFIR profiles and IOC hunting (v0.4.0)

- [ ] `velo_validate_dfir_profile` correctly flags a profile referencing
      a non-allowlisted artifact.
- [ ] `velo_collect_dfir_profile_with_approval` /
      `velo_start_dfir_hunt_with_approval` expand to exactly the
      profile's artifact list, no more.
- [ ] `velo_hunt_ioc_with_approval` rejects malformed
      hash/IP/domain input (fuzz with edge cases: IPv6, defanged
      indicators like `1[.]2[.]3[.]4`, mixed-case hashes, trailing dot
      domains).

### Phase 6 — negative/adversarial testing (v0.5.0)

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
