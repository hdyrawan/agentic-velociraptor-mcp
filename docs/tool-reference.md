# Tool reference

Status: this describes the **planned** stable core of 27 tools. As of
v0.3.0, exactly 11 are implemented and registered as callable MCP tools:
`velo_health_check`, `velo_search_clients`, `velo_get_client_info`,
`velo_list_artifact_names`, `velo_get_artifact_details`,
`velo_list_dfir_profiles`, `velo_get_dfir_profile`, and
`velo_validate_dfir_profile`, plus `velo_plan_dfir_triage`,
`velo_compare_dfir_profiles`, and `velo_find_profiles_by_artifact`. See
`internal/mcpserver/server.go`'s `New` function — only the visibility,
profile, and workflow registration functions are called; the flow,
collection, hunt, and IOC execution groups' `ToolSpec` metadata exists
for this document but is not wired to `mcp.AddTool` yet, and is therefore
not callable by any MCP client (confirmed by
`internal/mcpserver/server_test.go`'s exact-tool-inventory and
never-registers-unsafe-tools tests). Update the "Implemented" column as
each remaining tool actually lands.

Legend: RO = read-only, no approval. Approval = requires a matching
`approval.Decision` (see [approval-flow.md](approval-flow.md)) before any
Velociraptor call is made.

**Response envelope (v0.2.0+):** `velo_search_clients`,
`velo_get_client_info`, `velo_list_artifact_names`, and
`velo_get_artifact_details` each embed `internal/response.Result`, adding
a top-level `status` field (`"success"` / `"empty"` / `"not_found"` /
`"error"`) alongside their existing `mode`/data/`message` fields — see
docs/security-model.md's "Evidence honesty" section for the full
contract, including why `velo_health_check`'s own pre-existing `status`
field (`"ok"`/`"error"`) was left as-is rather than migrated. v0.3.0's
three workflow tools also embed the same `internal/response.Result`
envelope.

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
| `velo_list_flows` | RO | List flows for a client. | v0.1.0 | no |
| `velo_get_flow_status` | RO | State of one flow. | v0.1.0 | no |
| `velo_get_flow_results` | RO | Result rows for one flow, bounded. | v0.1.0 | no |
| `velo_list_flow_uploads` | RO | List uploads attached to a flow. | unscheduled (was v0.2.0; see PROJECT_PLAN.md's v0.2.0 re-scope note) | no |
| `velo_get_flow_upload_metadata` | RO | Metadata for one upload. | unscheduled (was v0.2.0; see PROJECT_PLAN.md's v0.2.0 re-scope note) | no |
| `velo_download_flow_upload_with_approval` | Approval | Download upload bytes, bounded by `max_upload_bytes`. | unscheduled (was v0.2.0; see PROJECT_PLAN.md's v0.2.0 re-scope note) | no |

## Collection tools (`tools_collection.go`)

| Tool | Kind | Description | Target milestone | Implemented |
|------|------|-------------|-------------------|-------------|
| `velo_collect_artifact_with_approval` | Approval | Collect one allowlisted artifact from one client. | unscheduled (was v0.2.0; see PROJECT_PLAN.md's v0.2.0 re-scope note) | no |
| `velo_collect_dfir_profile_with_approval` | Approval | Collect every artifact in an allowlisted DFIR profile from one client. | unscheduled (was v0.2.0; see PROJECT_PLAN.md's v0.2.0 re-scope note) | no |
| `velo_cancel_flow_with_approval` | Approval | Cancel a running flow. | unscheduled (was v0.2.0; see PROJECT_PLAN.md's v0.2.0 re-scope note) | no |

## Hunt tools (`tools_hunts.go`)

| Tool | Kind | Description | Target milestone | Implemented |
|------|------|-------------|-------------------|-------------|
| `velo_preview_hunt_scope` | RO | Resolve a proposed scope against the live client population. | v0.3.0 | no |
| `velo_start_hunt_with_approval` | Approval | Start a hunt for one allowlisted artifact. | v0.3.0 | no |
| `velo_start_dfir_hunt_with_approval` | Approval | Start a hunt for a DFIR profile's artifacts. | v0.3.0 | no |
| `velo_list_hunts` | RO | List hunts. | v0.3.0 | no |
| `velo_get_hunt_status` | RO | State/client count of one hunt. | v0.3.0 | no |
| `velo_get_hunt_results` | RO | Result rows for one hunt, bounded. | v0.3.0 | no |
| `velo_cancel_hunt_with_approval` | Approval | Stop a running hunt. | v0.3.0 | no |

## DFIR profile tools (`tools_profiles.go`)

| Tool | Kind | Description | Target milestone | Implemented |
|------|------|-------------|-------------------|-------------|
| `velo_list_dfir_profiles` | RO | List DFIR profiles loaded from the profiles directory. | v0.4.0 (moved up to v0.1.0-alpha.1) | **yes** |
| `velo_get_dfir_profile` | RO | Full definition of one profile; safe structured error if the name doesn't exist. | v0.4.0 (moved up to v0.1.0-alpha.1) | **yes** |
| `velo_validate_dfir_profile` | RO | Validate a profile's artifacts against the current policy artifact allowlist. | v0.4.0 (moved up to v0.1.0-alpha.1) | **yes** |

Note: `velo_list_dfir_profiles` and `velo_get_dfir_profile` return every
profile loaded from the profiles directory, not filtered by
`policy.allowed_profiles` — reading a profile *definition* is not
sensitive (it's a reviewed, versioned file, not endpoint data), so it is
not allowlist-gated. `policy.allowed_profiles` is enforced at the point a
profile is actually *used* (`velo_collect_dfir_profile_with_approval` /
`velo_start_dfir_hunt_with_approval`, both still unimplemented). Only
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
| `velo_hunt_ioc_with_approval` | Approval | Hunt for a validated hash/IP/domain using a fixed template. | v0.4.0 | no |

## Explicitly not in the stable core

- `run_vql` / any raw-VQL tool — see
  [security-model.md](security-model.md).
- Any generic remote shell / command-execution tool.
