# Live validation report — v0.10.2

Status: complete. This is a validation-only milestone: no MCP tool was
added or removed, no raw VQL/generic query path was introduced, no
approval gate, policy allowlist, audit behavior, or fail-closed default
was weakened. One real bug was found and fixed (see "Bugs found and
fixed" below); everything else in this report is validation findings
and documentation.

> **v0.10.3 update**: the two bugs documented below as "found — not
> fixed" (named-source result retrieval; the IOC-hunt artifact mapping)
> were fixed in v0.10.3. See CHANGELOG.md's v0.10.3 entry and
> [docs/tool-reference.md](tool-reference.md)'s "Named result sources"
> and "IOC kind support status" sections for the current, fixed
> behavior. The findings below are left unchanged as the historical
> record of what this pass actually observed.

- **Commit tested**: `f7479b916cd066c8f17c001490f6c809cd90f770` (`f7479b9`,
  the tip of `master` at the start of this validation pass — v0.10.1
  stabilization).
- **Validation date**: 2026-07-07.
- **Lab**: disposable, Docker-based, never connected to any production
  fleet. Torn down at the end of this pass.

## Lab environment summary

| Component | Detail |
|---|---|
| Server | `wlambert/velociraptor:latest` container, **Velociraptor 0.76.3** (commit `1c6f3b700`, built 2026-04-15), isolated Docker network/volume, ports published to `127.0.0.1` only. |
| Clients | One disposable Linux client (Ubuntu 22.04 container running the server's own repacked `velociraptor_client` binary), enrolled as `C.ffee25431ffb5019`, hostname `mcp-lab-linux-01`. **No Windows client** — a Windows VM/container was not practical in this environment; see "Limitations" below. |
| Artifact catalog | 433 artifacts (`velociraptor artifacts list`), consistent with the count recorded in `catalog/artifacts.yaml` (v0.10.0, same image). |
| API identities | Two least-privilege Velociraptor API clients generated via `velociraptor config api_client`, role-based (not `administrator`): **`reader`** role → this project's `read_api_config_path`; **`investigator`** role → `write_api_config_path`. Neither holds `administrator`, `artifact_writer`, or `server_admin`, matching [velociraptor-permissions.md](velociraptor-permissions.md). |
| Test label | `mcp-lab` (attempted; see "Limitations — label application" below for why label-scoped hunt matching could not be fully confirmed against a genuinely labeled client in this pass). |
| MCP server | Built from the commit above via `go build -buildvcs=false`, run over **stdio only**, driven by `npx @modelcontextprotocol/inspector --cli` against the real built binary (a real subprocess, not an in-memory test transport). |

## Redacted config summary

The lab config followed [configuration.md](configuration.md)'s schema
exactly, `policy.mode: controlled`, generic paths substituted below (the
real config used absolute scratch paths, never committed):

```yaml
server:
  transport: stdio

velociraptor:
  org_id: root
  read_api_config_path: /path/to/mcp-lab-reader.api.config.yaml
  write_api_config_path: /path/to/mcp-lab-investigator.api.config.yaml
  timeout_seconds: 30
  max_rows: 500
  max_result_bytes: 1048576
  max_upload_bytes: 52428800
  download_dir: /path/to/downloads

policy:
  mode: controlled
  allow_raw_vql: false
  allow_list_all_artifacts: false
  allow_target_all: true   # tiny 1-client lab only; see hunt validation below
  max_hunt_clients: 5
  require_approval_for: [collect_artifact, collect_dfir_profile, start_hunt,
    start_dfir_hunt, cancel_flow, cancel_hunt, download_flow_upload, hunt_ioc]
  allowed_artifacts:
    - Generic.Client.Info
    - Windows.System.Pslist
    - Linux.Sys.Pslist
    - Generic.System.Pstree
    - Windows.Network.Netstat
    - Linux.Network.Netstat
    - System.Hash.Hunt          # added to reach the IOC-hunt backend call; see finding below
    - Linux.Search.FileFinder   # added for an upload-path attempt; see finding below
  allowed_profiles: [windows_basic_triage, linux_basic_triage]

audit:
  enabled: true
  path: /path/to/audit.jsonl

approval:
  store_path: /path/to/approvals.json
  ttl_seconds: 900
```

No secrets, API keys, certs, Velociraptor configs, client configs, or
machine-specific paths are committed anywhere in this repository as part
of this validation; all of the above lived under a session scratch
directory outside the repo and was discarded after this pass.

## Build and inventory

- `go build -buildvcs=false ./...` — **pass**.
- MCP server run via **stdio only** (`--config ... --profiles-dir profiles`,
  no other transport flag exists).
- `ListTools` via MCP Inspector against the live subprocess: **exactly 28
  tools** — matches `internal/mcpserver/server_test.go`'s exact-inventory
  test and the previous mock-mode Inspector smoke test.
- **No raw VQL / generic query tool**: confirmed both by tool-name
  inspection (no `query`/`vql`-named tool in the 28) and by re-confirming
  `grpcClient` has no generic `Query` method (matches the 2026-07-06
  finding, unchanged).

## Pass/fail table

| Area | Tool / workflow | Result |
|---|---|---|
| Visibility | `velo_health_check` | **PASS** — `mode: real`, `velociraptor_connected: true`. |
| Visibility | `velo_search_clients` | **PASS** — real client returned with correct hostname/OS/agent_version/last_seen/last_ip. |
| Visibility | `velo_get_client_info` | **PASS** — real client fields incl. MAC address; nonexistent client ID correctly returns `status: not_found` (confirms the 2026-07-06 `ErrClientNotFound` fix holds). |
| Visibility | `velo_list_artifact_names` | **PASS** — allowlist-scoped list matches configured `allowed_artifacts` exactly. |
| Visibility | `velo_get_artifact_details` | **PASS** — allowed artifact returns real description/no VQL body; non-allowlisted artifact blocked (`blocked`, not silently empty). |
| DFIR workflow | `velo_list_dfir_profiles` | **PASS** — 46 profiles. |
| DFIR workflow | `velo_get_dfir_profile` | **PASS**. |
| DFIR workflow | `velo_validate_dfir_profile` | **PASS** — both allowlist-valid and allowlist-violating cases correctly reported. |
| DFIR workflow | `velo_plan_dfir_triage` | **PASS**. |
| DFIR workflow | `velo_compare_dfir_profiles` | **PASS after fix** — see "Bugs found and fixed". |
| DFIR workflow | `velo_find_profiles_by_artifact` | **PASS**. |
| Collection | `velo_collect_artifact_with_approval` (no approval) | **PASS** — `not_found`, no execution. |
| Collection | `velo_collect_artifact_with_approval` (approved) | **PASS** — real `CollectArtifact` RPC, flow created, approval consumed, reuse blocked (`"already been used"`). |
| Collection | `velo_list_flows` / `velo_get_flow_status` | **PASS** — real data, correct state (`finished`). |
| Collection | `velo_get_flow_results` | **PARTIAL** — works correctly for default-source artifacts (`Linux.Sys.Pslist`: real rows, correct fields); returns a false `empty` for `Generic.Client.Info` (named-source artifact). See "Bugs found — not fixed" below. |
| Collection | `velo_list_flow_uploads` / `velo_get_flow_upload_metadata` | **PASS (honest-empty)** — no allowlisted artifact in this pass produced an upload; a `Linux.Search.FileFinder` attempt targeting `/etc/hostname` matched 0 files, so the download tool itself was not exercised against a real upload. Not a code defect. |
| Collection | `velo_cancel_flow_with_approval` | Not exercised this pass (no long-running flow to cancel; time-boxed). |
| Hunts | `velo_preview_hunt_scope` (label) | **PASS (honest)** — correctly reported 0 matched clients because no client actually carried the test label in this pass (lab-tooling limitation, not a code defect — see "Limitations"). |
| Hunts | `velo_preview_hunt_scope` (all) | **PASS** — `matched: 1` in the 1-client lab. |
| Hunts | `velo_start_dfir_hunt_with_approval` (label scope) | **PASS (structural)** — real `CreateHunt` RPC succeeded, correct per-profile-artifact hunts created, condition correctly stored server-side (`condition.labels.label: ["mcp-lab"]`); 0 clients scheduled for the reason above. |
| Hunts | `velo_start_hunt_with_approval` (all-clients scope) | **PASS (full)** — real `CreateHunt`, `EstimateHunt` correctly showed `matched: 1`, and the client was genuinely scheduled: a hunt-driven flow (`F.*.H` suffix) appeared on the client and `velo_get_hunt_status` reported `client_count: 1`. This is full end-to-end confirmation of hunt creation → scheduling → client execution against a real server. |
| Hunts | `velo_list_hunts` / `velo_get_hunt_status` | **PASS** — real data. |
| Hunts | `velo_get_hunt_results` | **PARTIAL** — same named-source limitation as `velo_get_flow_results` (the all-clients hunt's `Generic.Client.Info` artifact returned `empty` despite `client_count: 1`). |
| Hunts | `velo_cancel_hunt_with_approval` | **PASS** — no-approval blocked; approved cancellation succeeded; `velo_get_hunt_status` correctly reflected `stopped` afterward. |
| Hunts | `velo_hunt_ioc_with_approval` (label scope, harmless test hash `d41d8cd98f00b204e9800998ecf8427e`) | **FAIL (confirmed pre-existing limitation)** — real `CreateHunt` call failed: `Unknown artifact System.Hash.Hunt`. Approval was still correctly consumed (one-shot semantics apply even on failure). Confirms the documented "illustrative/unverified IOC artifact names" caveat is not just unconfirmed but **definitively non-functional** against a real 433-artifact catalog. |
| Unsupported scope | `velo_start_hunt_with_approval` explicit `client_ids` | **PASS** — clear structured error, approval left `consumed: false`. |
| Unsupported scope | `velo_start_dfir_hunt_with_approval` explicit `client_ids` | **PASS** — same. |
| Unsupported scope | `velo_hunt_ioc_with_approval` explicit `client_ids` | **PASS** — same. |
| MCP Inspector | `tools/list` against real lab | **PASS** — exactly 28 tools via a real stdio subprocess. |
| MCP Inspector | Read-only tool calls | **PASS** — see visibility/DFIR rows above, all driven through Inspector's `--cli` mode. |
| MCP Inspector | Write tool without approval | **PASS** — refused (`not_found`), consistent with the existing mock-mode smoke test. |

## Approval/audit observations

- Every approval-gated call in this pass produced the documented
  **two-phase audit trail**: an `attempt` event before the Velociraptor
  call, then a `success`/`error`/`blocked` event after. 58 total audit
  events: 42 `success`, 7 `blocked`, 7 `attempt`, 2 `error`. No secret
  material (`BEGIN CERTIFICATE`/`BEGIN ... PRIVATE KEY` or similar)
  appeared anywhere in the audit log.
- **Approval consumption confirmed both on success and on failure**: a
  successful collection consumed its approval and rejected reuse
  (`"approval reference has already been used"`); the IOC hunt's failed
  `CreateHunt` call (nonexistent artifact) still consumed its approval —
  matching the documented one-shot-attempt semantics in
  [approval-flow.md](approval-flow.md).
- **Explicit `client_ids` hunt scope preserves the approval** for all
  three affected tools (`velo_start_hunt_with_approval`,
  `velo_start_dfir_hunt_with_approval`, `velo_hunt_ioc_with_approval`) —
  confirmed directly against the approval store JSON (`"consumed": false`
  after the refusal), not just inferred from the tool's response.
- No MCP tool call was able to create or approve its own request; every
  approval in this pass was created via the separate `approve` CLI
  subcommand, run directly against the same store file, never through the
  MCP stdio transport.

## Bugs found and fixed

1. **`velo_compare_dfir_profiles` duplicated common-artifact entries.**
   `internal/mcpserver/tools_workflow.go`'s common-artifact computation
   iterated per-profile-per-artifact, appending an artifact once for
   *each* profile that contained it rather than once overall — comparing
   `windows_basic_triage` and `linux_basic_triage` (which share
   `Generic.Client.Info`) returned
   `"common_artifacts": ["Generic.Client.Info", "Generic.Client.Info"]`.
   Fixed by computing `common` once from `artifactCounts` directly,
   independent of the per-profile loop. Added
   `TestCompareDFIRProfilesCommonArtifactsHasNoDuplicates`
   (`internal/mcpserver/tools_workflow_test.go`) and re-verified live
   against the real lab after rebuilding — `common_artifacts` now
   correctly returns `["Generic.Client.Info"]`. Cosmetic/correctness
   only; no security implication (this is a read-only, local, non-approval
   tool).

## Bugs found — not fixed (documented as confirmed limitations)

2. **`velo_get_flow_results` / `velo_get_hunt_results` silently report
   `empty` for artifacts with a named (non-default) Velociraptor
   source**, most notably **`Generic.Client.Info`** — used by nearly
   every DFIR profile. Root cause: `GetTable`'s `artifact` field must be
   source-qualified (`ArtifactName/SourceName`) for artifacts compiling
   to more than one named source; this project's `firstArtifactName`
   helper (`internal/velociraptor/grpcclient_flows.go`) only ever
   recovers the bare artifact name from the flow's stored request, and
   this project's `veloapi.Artifact` message deliberately carries no
   source-name metadata (only `name`/`description`/`parameters`, by
   design — to keep VQL query bodies structurally unreachable). Confirmed
   directly: the real collection's on-disk storage
   (`artifacts/Generic.Client.Info/<flow>/BasicInformation.json`, plus
   `DetailedInfo.json`/`LinuxInfo.json`) holds real data the tool cannot
   retrieve, while a default-source artifact (`Linux.Sys.Pslist`) returns
   correct real rows through the same code path. **Not fixed in this
   pass**: a correct fix requires adding source-name (not query-body)
   metadata to the `Artifact` proto/type and threading it through both
   `GetFlowResults` and `GetHuntResults` — a schema change with its own
   test surface, out of scope for a "validation only, minimal fix"
   release. This is an evidence-honesty concern (a real, successful
   collection currently looks like it produced nothing) and should be the
   top priority for the next feature milestone.
3. **The IOC-hunt artifact mapping (`System.Hash.Hunt`/`System.IP.Hunt`/
   `System.Domain.Hunt`/`System.Process.Hunt`/`System.Path.Hunt`) does not
   exist in any real Velociraptor catalog.** Previously documented as
   "illustrative/unverified"; this pass converts that to a **confirmed,
   reproducible failure** (`Unknown artifact System.Hash.Hunt` from a
   real `CreateHunt` call). `velo_hunt_ioc_with_approval` cannot succeed
   against a real server until `internal/vql.Bind`'s template mapping is
   updated to real, catalog-verified artifact/parameter names — the same
   caveat the pre-existing `ioc_hash_hunt`/`ioc_ip_hunt`/`ioc_domain_hunt`
   DFIR profiles already carried (see docs/dfir-profiles.md). Not fixed
   here: choosing correct replacement artifacts and parameter bindings is
   a design decision, not a minimal validation-pass fix.

## Limitations of this pass

- **Label application could not be reliably confirmed.** Multiple
  attempts to apply the `mcp-lab` label to the test client — via the
  Velociraptor CLI's local `query` subcommand, via the live gRPC Query API
  under both `investigator` and `reader` identities, and via the GUI's
  REST endpoint (blocked by CSRF token handling not scriptable in the
  time available) — did not result in a label persisting on the client
  (`labels: []` on every re-check). This is judged a **lab-tooling
  limitation**, not a defect in this project: (a) the earlier apparent
  `velo_search_clients` "label match" was actually a hostname-substring
  match (`mcp-lab-linux-01` contains `mcp-lab`), not a real label match;
  (b) `velo_preview_hunt_scope`/`velo_start_dfir_hunt_with_approval`
  correctly and honestly reported 0 matched/scheduled clients once this
  was understood, and the same code path was then proven fully correct
  end-to-end using the `all`-clients scope instead (see the hunts row
  above) — the label **condition** this project constructs and sends to
  Velociraptor was independently confirmed correct by inspecting the
  hunt's stored server-side JSON (`condition.labels.label: ["mcp-lab"]`).
  A genuinely label-scoped hunt matching a labeled client was not
  demonstrated in this pass; the underlying mechanism was demonstrated
  correct via the equivalent all-clients path instead.
- **No Windows client.** Only a disposable Linux client was available in
  this Docker-only environment; Windows-specific artifacts/tools were
  validated only for allowlist/schema correctness, not live execution.
- **No upload/download exercised against real evidence bytes.** No
  allowlisted-and-approved collection in this pass produced a file
  upload (`Generic.Client.Info`/`Linux.Sys.Pslist` are metadata-only; a
  `Linux.Search.FileFinder` attempt matched 0 files). `velo_list_flow_uploads`
  was confirmed to report an honest empty list, but
  `velo_download_flow_upload_with_approval` and `max_upload_bytes`
  enforcement were not exercised against a real upload.
- **`velo_cancel_flow_with_approval`** was not exercised (no long-running
  flow existed to cancel within the time available for this pass).
- **`allow_target_all: true` was used deliberately** in this pass, in a
  genuinely tiny (1-client) disposable lab, exactly as the validation
  scope for this milestone permits — this is not a recommendation to
  change the default (`false`) for any real deployment.

## Production-readiness gaps (carried forward)

- Fix the named-source result-retrieval gap (finding 2 above) before
  relying on `velo_get_flow_results`/`velo_get_hunt_results` for
  `Generic.Client.Info` or any other multi-source artifact in a real
  investigation.
- Do not rely on `velo_hunt_ioc_with_approval` against a real server
  until its artifact/parameter mapping is corrected (finding 3 above).
- Confirm label-based hunt/collection scoping against a client with a
  verified, persistent label before depending on it operationally — this
  pass could not produce one to test against.
- Validate against a Windows client before depending on Windows-specific
  DFIR profiles/artifacts in production.
- Exercise the upload/download path (`velo_list_flow_uploads` /
  `velo_get_flow_upload_metadata` / `velo_download_flow_upload_with_approval`)
  against a real file-producing collection, including `max_upload_bytes`
  enforcement.
- Everything else in `docs/lab-validation-plan.md`'s remaining unchecked
  items (Phase 5's `velo_cancel_flow_with_approval`; Phase 8's adversarial
  testing) is still open.

## Summary

- **Inventory remains exactly 28 tools; no raw VQL, generic query, or new
  write path was added.**
- **Real gRPC RPCs confirmed live and working**: `Check`, `ListClients`,
  `GetClient`, `GetArtifacts` (all reconfirmed from 2026-07-06),
  `CollectArtifact`, `GetFlowDetails`, `GetTable` (default-source case),
  `CreateHunt`, `EstimateHunt`, `CancelHunt`, `ListHunts`. This is the
  first live confirmation of the entire collection and hunt RPC groups
  that v0.10.1's "Backend wiring status" table listed as "not yet
  live-validated."
- **Approval/audit/policy controls held throughout**: no approval-gated
  operation executed without a valid, fingerprint-matched, unconsumed
  approval; the explicit-`client_ids` hunt-scope gate preserved every
  approval it refused; no secret material leaked into the audit log.
- **One real bug found and fixed** (duplicate `common_artifacts`); **two
  real bugs found and documented, not fixed** (named-source result
  retrieval; nonexistent IOC-hunt artifacts) — both are correctness/
  capability gaps, not security-control weaknesses, and both are called
  out as required follow-up before relying on the affected tools in
  production.
