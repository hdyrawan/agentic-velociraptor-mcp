# Tool reference

As of v0.10.1, all 28 tools — the full stable-core target — are
implemented and registered as callable MCP tools: 5 visibility (`velo_health_check`,
`velo_search_clients`, `velo_get_client_info`, `velo_list_artifact_names`,
`velo_get_artifact_details`), 3 DFIR profile (`velo_list_dfir_profiles`,
`velo_get_dfir_profile`, `velo_validate_dfir_profile`), 3 DFIR workflow
(`velo_plan_dfir_triage`, `velo_compare_dfir_profiles`,
`velo_find_profiles_by_artifact`), 3 flow/result (`velo_list_flows`,
`velo_get_flow_status`, `velo_get_flow_results`), 6 collection/flow-upload
(`velo_collect_artifact_with_approval`,
`velo_collect_dfir_profile_with_approval`,
`velo_cancel_flow_with_approval`, `velo_list_flow_uploads`,
`velo_get_flow_upload_metadata`,
`velo_download_flow_upload_with_approval`), 7 hunt management
(`velo_preview_hunt_scope`, `velo_start_hunt_with_approval`,
`velo_start_dfir_hunt_with_approval`, `velo_list_hunts`,
`velo_get_hunt_status`, `velo_get_hunt_results`,
`velo_cancel_hunt_with_approval`), and 1 IOC helper
(`velo_hunt_ioc_with_approval`). Every tool listed below is registered
via `mcp.AddTool`; there is no remaining unwired `ToolSpec` metadata.

Legend: RO = read-only, no approval. Approval = requires a resolvable
`approval_reference` (see [approval-flow.md](approval-flow.md)) before
any Velociraptor call is made. **This remains a controlled pilot, not
unrestricted Velociraptor write access**: every Approval-kind tool below
requires `policy.mode: controlled` and `approval.store_path` to be
explicitly configured (off by default), and no raw-VQL tool exists
anywhere in this codebase.

**Response envelope (v0.2.0+):** `velo_search_clients`,
`velo_get_client_info`, `velo_list_artifact_names`, and
`velo_get_artifact_details` each embed `internal/response.Result`, adding
a top-level `status` field (`"success"` / `"empty"` / `"not_found"` /
`"error"`) alongside their existing `mode`/data/`message` fields — see
docs/security-model.md's "Evidence honesty" section for the full
contract, including why `velo_health_check`'s own pre-existing `status`
field (`"ok"`/`"error"`) was left as-is rather than migrated. v0.3.0's
three workflow tools, v0.5.0's three flow/result tools, and every v0.4.0
tool also embed the same `internal/response.Result` envelope.

**Approval-gated tool inputs:** every Approval-kind tool takes `case_id`,
`reason`, `requester`, its target (client plus
artifact/profile/flow_id/upload_name/scope as applicable), and an
`approval_reference`. The reference must name an
`internal/approval.Store` record — created and approved out-of-band by a
human operator via the `agentic-velociraptor-mcp approve` CLI subcommand,
never by any MCP tool — that is approved, unconsumed, unexpired, and
whose `approval.RequestFingerprint` exactly matches the call's operation/
case_id/client_id/artifact/profile/parameters/flow_id/upload_name/scope.
Backend-capability and scope-support checks happen before approval
consumption, so a request the configured backend cannot execute (an
unimplemented operation, or an explicit `client_ids` hunt scope in real
mode — see "Known real-mode limitation" below) preserves the approval for
a later, valid retry instead of burning it. See
[approval-flow.md](approval-flow.md) for the full operator workflow.

## Visibility tools (`tools_visibility.go`)

| Tool | Kind | Description | Target milestone | Implemented |
|------|------|-------------|-------------------|-------------|
| `velo_health_check` | RO | Connectivity check against the read API via Velociraptor's dedicated `Check` gRPC RPC. Runs in mock mode (no Velociraptor call) when `velociraptor.read_api_config_path` is unset. | v0.1.0-alpha.1 (static, done) / v0.1.0-alpha.2 (real, done) | **yes (mock or real)** |
| `velo_search_clients` | RO | Search clients by hostname/IP/label substring/glob via Velociraptor's `ListClients` gRPC RPC. `query` and `limit` are optional; results bounded by `velociraptor.max_rows`. | v0.1.0 | **yes (mock or real)** |
| `velo_get_client_info` | RO | Detail (hostname, OS, last seen, labels, MAC addresses) for one already-identified client ID via `GetClient`. `client_id` validated before any call. | v0.1.0 | **yes (mock or real)** |
| `velo_list_artifact_names` | RO | List artifact names/descriptions via `GetArtifacts`; restricted to `policy.allowed_artifacts` unless `policy.allow_list_all_artifacts` is set. | v0.1.0 | **yes (mock or real)** |
| `velo_get_artifact_details` | RO | Parameter schema (never the VQL body) for one artifact via `GetArtifacts` filtered by exact name. `name` validated and allowlist-gated the same as `velo_list_artifact_names`. | v0.1.0 | **yes (mock or real)** |

## Flow and result tools (`tools_flows.go`)

| Tool | Kind | Description | Target milestone | Implemented |
|------|------|-------------|-------------------|-------------|
| `velo_list_flows` | RO | List flows for a client, bounded by `max_rows` with cursor pagination. | v0.1.0 / v0.5.0 backfill | **yes (mock, or real via `GetClientFlows`)** |
| `velo_get_flow_status` | RO | State of one flow. `client_id` and `flow_id` are validated before any call. | v0.1.0 / v0.5.0 backfill | **yes (mock, or real via `GetFlowDetails`)** |
| `velo_get_flow_results` | RO | Result rows for one flow, bounded by `max_rows` and `max_result_bytes`; reports `truncated`, `returned_rows`, `byte_count`, and optional `next_cursor`. Optional `source` input selects a named Velociraptor result source (see below). | v0.1.0 / v0.5.0 backfill | **yes (mock, or real via `GetTable`)** |
| `velo_list_flow_uploads` | RO | List uploads attached to a flow via `Deps.ReadClient`; same mock/real convention as the visibility tools. | v0.4.0 | **yes (mock or real)** |
| `velo_get_flow_upload_metadata` | RO | Size/hash metadata for one upload; `status: "not_found"` via `velociraptor.ErrUploadNotFound` for an unknown upload. | v0.4.0 | **yes (mock or real)** |
| `velo_download_flow_upload_with_approval` | Approval | Download upload bytes (bounded by `max_upload_bytes`) and write them to a local `velociraptor.download_dir` file; the MCP response never carries raw bytes, only `local_path`/`size_bytes`/`sha256`. Requires `velociraptor.download_dir` configured in addition to the standard write-pilot gate. | v0.4.0 | **yes (mock, or real via `VFSGetBuffer`)** |


### Flow/result response contract

The three flow/result tools are read-only. They never collect, cancel,
download, mutate client/server state, or expose raw VQL. Malformed
`client_id` / `flow_id` input is blocked before any backend call and
audited as `blocked`. Mock mode returns empty data with a `success`
status and explicit `mode: "mock"`. Real mode calls `Deps.ReadClient`'s
typed gRPC methods (`GetClientFlows`/`GetFlowDetails`/`GetTable`) against
the real server; a connectivity or lookup failure is reported as a
normal structured `error` result, not a Go-level panic. Tests exercise
the handler contract against both a fake read client and, at the
`internal/velociraptor` layer, a fake gRPC service stub.

`velo_get_flow_results` always applies the lower of the requested
`limit` and configured `velociraptor.max_rows`, then bounds serialized
row payload size by `velociraptor.max_result_bytes`. Partial responses
set `truncated: true` and include `next_cursor` when another page can be
requested. Audit events include `client_id`, `flow_id`, `row_count`, and
`byte_count`.

### Named result sources (v0.10.3)

Some Velociraptor artifacts compile to more than one named result
source — most notably **`Generic.Client.Info`** (`BasicInformation`/
`DetailedInfo`/`LinuxInfo`), used by nearly every DFIR profile. Real
Velociraptor's `GetTable`/`GetHuntResults` RPCs need a *source-qualified*
name (`ArtifactName/SourceName`) to read such an artifact's result table
— the bare artifact name only ever addresses a table for an artifact
whose single source has no explicit name (e.g. `Linux.Sys.Pslist`,
`Windows.System.Pslist`).

Before v0.10.3, `velo_get_flow_results`/`velo_get_hunt_results` always
queried the bare name, so any multi-source artifact silently returned
zero rows even though the collection succeeded and real data existed —
see docs/live-validation-report-v0.10.2.md finding 2. As of v0.10.3:

- **Single-source artifacts are unaffected** — no input change needed;
  results are retrieved exactly as before.
- **Single *named*-source artifacts** (a source with an explicit name,
  but only one such source) have that source selected automatically,
  transparently fixing the previously-silent-empty case.
- **Multi-source artifacts with no `source` specified** return
  `status: "source_required"` with a real `available_sources` array
  (fetched from Velociraptor's own `GetArtifacts` RPC) instead of an
  empty result — a normal, actionable structured response, not an error.
- **An explicit `source` input** (both tools accept an optional
  `source` string) is validated against the artifact's real declared
  source names; an unrecognized source is rejected with a clear error
  rather than silently querying a nonexistent table.

This is backwards-compatible: no existing caller that never set `source`
and only ever collected single-source artifacts sees any behavior
change.

## Collection tools (`tools_collection.go`)

| Tool | Kind | Description | Target milestone | Implemented |
|------|------|-------------|-------------------|-------------|
| `velo_collect_artifact_with_approval` | Approval | Collect one allowlisted artifact from one client, with optional agent-supplied `parameters`. | v0.4.0 | **yes (mock, or real via `CollectArtifact`)** |
| `velo_collect_dfir_profile_with_approval` | Approval | Collect every artifact in an allowlisted, locally-loaded DFIR profile from one client, using each artifact's own fixed profile parameters (no agent-supplied parameters). Reports partial progress (`flows`) if collection stops partway through. | v0.4.0 | **yes (mock, or real via `CollectArtifact`)** |
| `velo_cancel_flow_with_approval` | Approval | Cancel a running flow. | v0.4.0 | **yes (mock, or real via `CancelFlow`)** |

These three tools, plus flow uploads and hunt start/cancel, call real
typed Velociraptor gRPC RPCs (`internal/velociraptor/grpcclient_flows.go`,
`grpcclient_uploads.go`, `grpcclient_hunts.go`) when
`velociraptor.write_api_config_path` is configured. A write client that
cannot perform a given operation (the built-in placeholder client used
when no write API config is set) reports a structured
`backend_not_implemented`/`error`-status response without consuming the
approval — see `internal/mcpserver/server.go`'s `backendOperationReady`.

## Hunt tools (`tools_hunts.go`)

| Tool | Kind | Description | Target milestone | Implemented |
|------|------|-------------|-------------------|-------------|
| `velo_preview_hunt_scope` | RO | Resolve a proposed scope against the live client population. Blocks `target_all` by default. | v0.6.0 | **yes** (mock, or real via `EstimateHunt`; explicit `client_ids` unsupported in real mode — see below) |
| `velo_start_hunt_with_approval` | Approval | Start a hunt for one allowlisted artifact. Enforces `max_hunt_clients`, artifact allowlist, scope validation. | v0.6.0 | **yes** (mock, or real via `CreateHunt`; explicit `client_ids` unsupported in real mode — see below) |
| `velo_start_dfir_hunt_with_approval` | Approval | Start a hunt for a DFIR profile's artifacts. Enforces profile allowlist, artifact allowlist, DFIR profile validation. | v0.6.0 | **yes** (mock, or real via `CreateHunt`; explicit `client_ids` unsupported in real mode — see below) |
| `velo_list_hunts` | RO | List hunts with cursor pagination. | v0.6.0 | **yes** (mock, or real via `ListHunts`) |
| `velo_get_hunt_status` | RO | State/client count of one hunt. Returns `not_found` for unknown hunt IDs. | v0.6.0 | **yes** (mock, or real via `GetHunt`) |
| `velo_get_hunt_results` | RO | Result rows for one hunt, bounded by `max_rows`/`max_result_bytes`, with cursor pagination. Optional `source` input selects a named Velociraptor result source — same behavior as `velo_get_flow_results`; see "Named result sources" above. | v0.6.0 | **yes** (mock, or real via `GetHuntResults`) |
| `velo_cancel_hunt_with_approval` | Approval | Stop a running hunt. | v0.6.0 | **yes** (mock, or real via `ModifyHunt`) |

### Known real-mode limitation: explicit `client_ids` hunt scope

Real Velociraptor's typed hunt RPCs (`CreateHunt`/`EstimateHunt`) accept a
`HuntCondition` that can express a label filter or "all clients" — there
is no field for an explicit, caller-chosen list of client IDs. So when
`velociraptor.write_api_config_path` (or the read equivalent for preview)
points at a real server, a hunt-scoped tool call with `client_ids` set
(`velo_preview_hunt_scope`, `velo_start_hunt_with_approval`,
`velo_start_dfir_hunt_with_approval`, `velo_hunt_ioc_with_approval`)
returns a structured error rather than attempting the RPC
(`velociraptor.ErrHuntScopeClientIDsUnsupported`). For the three
approval-gated tools, this check runs before the approval is consumed, so
the approval remains valid for a retry with `label` or `all` scope
instead. Label- and all-clients-scoped hunts are fully implemented
against the real RPCs. This has no effect in mock mode, and does not
affect single-client `client_id` tools like
`velo_collect_artifact_with_approval` (a different, unrelated field).

## DFIR profile tools (`tools_profiles.go`)

| Tool | Kind | Description | Target milestone | Implemented |
|------|------|-------------|-------------------|-------------|
| `velo_list_dfir_profiles` | RO | List DFIR profiles loaded from the profiles directory. | originally planned for v0.4.0, moved up to v0.1.0-alpha.1 | **yes** |
| `velo_get_dfir_profile` | RO | Full definition of one profile; safe structured error if the name doesn't exist. | originally planned for v0.4.0, moved up to v0.1.0-alpha.1 | **yes** |
| `velo_validate_dfir_profile` | RO | Validate a profile's artifacts against the current policy artifact allowlist. | originally planned for v0.4.0, moved up to v0.1.0-alpha.1 | **yes** |

Note: `velo_list_dfir_profiles` and `velo_get_dfir_profile` return every
profile loaded from the profiles directory, not filtered by
`policy.allowed_profiles` — reading a profile *definition* is not
sensitive (it's a reviewed, versioned file, not endpoint data), so it is
not allowlist-gated. `policy.allowed_profiles` is enforced at the point a
profile is actually *used* (`velo_collect_dfir_profile_with_approval`,
implemented in v0.4.0; `velo_start_dfir_hunt_with_approval`, implemented
in v0.6.0). Only
`velo_validate_dfir_profile` currently cross-checks against
`policy.allowed_artifacts` (not `allowed_profiles`), since validating
artifact allowlist membership is exactly what that tool is for.

## DFIR workflow tools (`tools_workflow.go`)

These v0.3.0 tools are read-only planning aids. They inspect only the
already-loaded local DFIR profile registry and local policy allowlists;
they do not make Velociraptor RPCs, do not execute collections, do not
start/cancel hunts, do not download evidence, and do not mutate endpoint
or server state.

| Tool | Kind | Description | Target milestone | Implemented |
|------|------|-------------|-------------------|-------------|
| `velo_plan_dfir_triage` | RO | Recommend profiles and read-only next steps for a `case_type`, optional `target_os`, and optional syntactically validated `client_id`. | v0.3.0 | **yes** |
| `velo_compare_dfir_profiles` | RO | Compare two to five loaded profiles by metadata, policy allowlist status, common artifacts, and per-profile unique artifacts. | v0.3.0 | **yes** |
| `velo_find_profiles_by_artifact` | RO | Return loaded profiles that reference an exact artifact name and whether that artifact is currently in `policy.allowed_artifacts`. | v0.3.0 | **yes** |

### `velo_plan_dfir_triage`

Input:

- `case_type` (optional): one of `basic`, `triage`, `process_network`,
  `persistence`, `lateral_movement`, `ransomware`, `credential_theft`,
  `eventlog`, `browser_activity`, `timeline`, or `ioc`.
- `target_os` (optional): `windows`, `linux`, or `any`.
- `client_id` (optional): validated as a Velociraptor client ID but never
  contacted.

Output embeds `status`/`message` and returns `recommendations`,
`read_only_next_steps`, and `safety_notes`. Recommendation entries include
profile metadata plus `allowed_by_policy`, `artifacts_allowlisted`, and an
optional `validation_error`. `status` is `empty` when no loaded profile
matches the filters. Invalid `case_type`, `target_os`, or `client_id`
returns a blocked tool error before any lookup.

Example:

```json
{"case_type":"ransomware","target_os":"windows","client_id":"C.1234abcd5678ef90"}
```

### `velo_compare_dfir_profiles`

Input:

- `names`: two to five DFIR profile names.

Output embeds `status`/`message` and returns `profiles`,
`common_artifacts`, `unique_artifacts`, and, for structured lookup
failures, `missing_profiles`. Unknown names return `status: "not_found"`
as a normal structured result; malformed names, duplicates, or too few /
too many names are blocked tool errors.

Example:

```json
{"names":["windows_basic_triage","windows_process_network_triage"]}
```

### `velo_find_profiles_by_artifact`

Input:

- `artifact`: exact Velociraptor artifact name.

Output embeds `status`/`message` and returns `artifact`,
`artifact_allowed`, and matching `profiles`. No match returns
`status: "not_found"`; malformed artifact syntax is a blocked tool
error.

Example:

```json
{"artifact":"Generic.Client.Info"}
```

## IOC helper tool (`tools_ioc.go`)

| Tool | Kind | Description | Target milestone | Implemented |
|------|------|-------------|-------------------|-------------|
| `velo_hunt_ioc_with_approval` | Approval | Hunt for a validated `hash`/`ip`/`domain`/`process`/`path` indicator using a fixed template, across a bounded scope. Enforces `max_hunt_clients`, artifact allowlist (on the template's resolved artifact), scope validation, `target_all` policy. As of v0.10.3, only `kind: "hash"` resolves to a real artifact — see below. | v0.7.0 | **partially** — see "IOC kind support status (v0.10.3)" below |

Input: `case_id`, `reason`, `requester`, `approval_id` (all required, same
as every other approval-gated tool), `kind` (one of `hash`, `ip`,
`domain`, `process`, `path`), `value` (the indicator, validated against
`kind` via `internal/validation.ValidateIOC` before it is ever bound into
a template parameter), plus the same scope fields as
`velo_start_hunt_with_approval` (`client_ids` / `label` / `all`,
`max_clients`). `kind`+`value` are resolved through
`internal/vql.Bind` to a fixed artifact name and a single bound parameter
— never a caller-chosen artifact, never string-interpolated VQL. The
resolved artifact must still pass `policy.allowed_artifacts` like any
other hunt target.

Output embeds `status`/`message` plus `mode`, `hunt_id`, `kind`,
`artifact` (the resolved artifact name, for auditability), `state`, and
`scope_desc`. A fingerprint-mismatched approval (e.g. approved for one
indicator value/scope, called for another) returns `status: "error"`
mentioning "does not match" with `status` unchanged by a Go-level error
(same convention as every other `verifyApproval/consumeApproval`-gated tool).

### IOC kind support status (v0.10.3)

v0.10.2's live-lab validation found that all five pre-v0.10.3 IOC
artifact mappings (`System.Hash.Hunt`/`System.IP.Hunt`/
`System.Domain.Hunt`/`System.Process.Hunt`/`System.Path.Hunt`) were
illustrative placeholders that do not exist in any real Velociraptor
catalog — a real `CreateHunt` call for any of them failed with
`Unknown artifact ...` (see docs/live-validation-report-v0.10.2.md
finding 3). v0.10.3 replaces this with real, catalog-verified coverage
where one exists, and an honest, pre-approval-consumption failure where
it doesn't:

| Kind | Status | Detail |
|------|--------|--------|
| `hash` | **Supported** | Resolves to `Generic.Detection.HashHunter` (confirmed present in a real Velociraptor 0.76.3 catalog and via a real `CreateHunt` call), with the indicator bound to whichever of its `MD5List`/`SHA1List`/`SHA256List` parameters matches the hash's own algorithm. |
| `ip` | **Unsupported** | No real, catalog-verified, cross-platform "hunt by IP" artifact was found that fits this project's one-artifact-per-template model. |
| `domain` | **Unsupported** | Same reasoning as `ip`. |
| `process` | **Unsupported** | Real process-listing artifacts (`Windows.System.Pslist`, `Linux.Sys.Pslist`, ...) take no process-name filter parameter; binding an indicator to one would silently return every process, not a real hunt for it. |
| `path` | **Unsupported** | Real per-OS FileFinder artifacts exist and do accept a path/glob parameter, but this project's model has no per-client-OS artifact selection, so a single choice would silently miss every other OS's clients. |

An unsupported kind fails with a clear error — naming that curated
artifacts are not yet installed for it — from `internal/vql.Bind`
(`ErrTemplateUnsupported`), called from `BuildHuntIOCApprovalRequest`
*before* the tool ever looks up or consumes an approval reference. This
means: an approval created (via the `approve` CLI) for an unsupported
kind cannot even be constructed — `BuildHuntIOCApprovalRequest` is the
same path both the CLI and the tool handler use — and an unsupported
call against an *existing* approval leaves it completely untouched
(`consumed: false`), auditable as `blocked`, never reaching the artifact
allowlist or write backend.

## Explicitly not in the stable core

- `run_vql` / any raw-VQL tool — see
  [security-model.md](security-model.md).
- Any generic remote shell / command-execution tool.
- Agent-supplied/mutable artifact or DFIR profile catalogs — see
  [dfir-profiles.md](dfir-profiles.md).

## Backend wiring status

The `internal/velociraptor/veloapi` proto mirror now includes reviewed
typed RPC bindings for flow enumeration/results (`GetClientFlows`,
`GetFlowDetails`, `GetTable`), collection execution and cancel
(`CollectArtifact`, `CancelFlow`), uploads (`VFSGetBuffer`), and hunt
execution/cancel/results (`CreateHunt`, `ModifyHunt`, `ListHunts`,
`GetHunt`, `GetHuntResults`, `EstimateHunt`) — see
`internal/velociraptor/grpcclient_flows.go`, `grpcclient_uploads.go`, and
`grpcclient_hunts.go`. None of this exposes a generic VQL query path,
which remains entirely out of the stable core.

| Group | Status |
|---|---|
| Visibility (`health`, client search/info, artifact list/details) | Real gRPC. |
| Flow list/status/results | Real gRPC (`GetClientFlows`/`GetFlowDetails`/`GetTable`). |
| Collection start / DFIR profile collection / flow cancel | Real gRPC (`CollectArtifact`/`CancelFlow`). |
| Flow uploads list/metadata/download | Real gRPC (`GetTable` for listing/metadata, `VFSGetBuffer` for download). |
| Hunts list/status/results/preview | Real gRPC (`ListHunts`/`GetHunt`/`GetHuntResults`/`EstimateHunt`); explicit `client_ids` scope unsupported (see above). |
| Approved hunt start/cancel and IOC hunt | Real gRPC (`CreateHunt`/`ModifyHunt`); explicit `client_ids` scope unsupported (see above). |

Every real-mode path above still requires a live, disposable Velociraptor
lab pass to confirm end-to-end behavior against enrolled endpoints and
real hunt/flow data before production use — see
[lab-validation-plan.md](lab-validation-plan.md) for what remains
unchecked.
