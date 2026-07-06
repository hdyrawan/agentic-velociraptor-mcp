# Configuration

Status: reflects `internal/config.Config` as of v0.1.0-alpha.2. This is
the authoritative structural reference; keep in sync with
`internal/config/config.go`.

**Requires Go 1.25+ to build** (the official MCP Go SDK dependency's
minimum; `go.mod`'s `go` directive was bumped from 1.23.4 in v0.1.0-alpha.1).

## File format

A single YAML file, path passed via `--config`. See
`internal/config.Load` / `internal/config.Validate`.

Note: the DFIR profile directory (`profiles/*.yaml`) is **not** part of
this YAML file — it's a separate `--profiles-dir` CLI flag (default
`profiles`), deliberately kept out of the config schema for this
milestone. See [dfir-profiles.md](dfir-profiles.md).

```yaml
server:
  name: agentic-velociraptor-mcp
  transport: stdio          # only "stdio" is supported

velociraptor:
  org_id: root
  read_api_config_path: /path/to/reader.api.config.yaml
  write_api_config_path: /path/to/investigator.api.config.yaml
  timeout_seconds: 30
  max_rows: 500
  max_result_bytes: 1048576
  max_upload_bytes: 52428800
  download_dir: /var/lib/agentic-velociraptor-mcp/downloads

policy:
  mode: read_only            # or "controlled"
  allow_raw_vql: false        # must stay false; validated
  allow_list_all_artifacts: false
  allow_target_all: false
  max_hunt_clients: 100
  require_approval_for:
    - collect_artifact
    - collect_dfir_profile
    - start_hunt
    - start_dfir_hunt
    - cancel_flow
    - cancel_hunt
    - download_flow_upload
  allowed_artifacts:
    - Generic.Client.Info
    - Windows.System.Pslist
    - Windows.Network.Netstat
  allowed_profiles:
    - windows_basic_triage
    - windows_ransomware_triage
    - linux_basic_triage

audit:
  enabled: true
  path: /var/log/agentic-velociraptor-mcp/audit.jsonl
  redact_fields:
    - client_private_key
    - client_cert
    - ca_certificate
    - api_key
    - approval_token
    - password
    - secret

approval:
  store_path: /var/lib/agentic-velociraptor-mcp/approvals.json
  ttl_seconds: 900
```

## Field reference

### `server`

| Field       | Type   | Notes                                   |
|-------------|--------|------------------------------------------|
| `name`      | string | Free-form server identity.                |
| `transport` | string | Must be `stdio`. HTTP/SSE only if/when explicitly requested (see PROJECT_PLAN.md). |

### `velociraptor`

| Field                   | Type   | Notes |
|-------------------------|--------|-------|
| `org_id`                | string | Velociraptor org to scope operations to. |
| `read_api_config_path`  | string | Path to a Velociraptor `api.config.yaml` (mTLS bundle) for a least-privilege, read-oriented identity. **Secret**: never logged. **Optional**: empty means every visibility tool (`velo_health_check`, `velo_search_clients`, `velo_get_client_info`, `velo_list_artifact_names`, `velo_get_artifact_details`) runs in mock mode (no Velociraptor call). If set, it is loaded eagerly at startup and must be valid — see "The read API config file" below — or the server refuses to start. |
| `write_api_config_path` | string | Separate `api.config.yaml` for an identity used only after approval for write-capable operations. **Secret**: never logged. Optional if the deployment is permanently read-only; if set, loaded eagerly at startup (must be valid) and used by the six v0.4.0 collection/cancel/download tools once `writePilotEnabled` conditions are met. Even so, this milestone's `veloapi` proto mirror has no real `CollectArtifact`/`CancelFlow`/upload RPC wiring yet — a real write client currently reports `ErrNotImplemented` for those calls. |
| `timeout_seconds`       | int    | Per-call timeout against the Velociraptor gRPC API, applied independently to each RPC (`Check`, `ListClients`, `GetClient`, `GetArtifacts`). Must be > 0. |
| `max_rows`              | int    | Max result rows returned by any single tool call. As of v0.1.0 this bounds `velo_search_clients` (caps both the requested `limit` and the server-reported result count) and `velo_list_artifact_names`. Must be > 0; a non-positive value falls back to an internal default of 100 rather than being treated as unbounded. |
| `max_result_bytes`      | int64  | Max serialized result size returned by any single tool call. Must be > 0. |
| `max_upload_bytes`      | int64  | Max bytes fetched by `velo_download_flow_upload_with_approval`'s underlying `DownloadFlowUpload` call. Must be > 0. |
| `download_dir`          | string | Local directory `velo_download_flow_upload_with_approval` writes downloaded evidence bytes to (never returned inline in the MCP response). **Optional; empty disables that one tool** even if `policy.mode`/`approval.store_path` are otherwise configured for the write pilot. Created (`0700`) on first use if it doesn't exist; files are written `0600`. |

### `policy`

| Field                      | Type       | Notes |
|----------------------------|------------|-------|
| `mode`                     | `read_only` \| `controlled` | `read_only` disables every write-capable tool regardless of the rest of this section. |
| `allow_raw_vql`            | bool       | Must be `false`. `config.Validate` rejects `true`. Exists so the restriction is explicit in config rather than only in code. |
| `allow_list_all_artifacts` | bool       | If true, `velo_list_artifact_names` may return the full server catalog. Does not affect what can be *collected* — that is still gated by `allowed_artifacts`. |
| `allow_target_all`         | bool       | Must stay `false` in normal operation. Gates whether hunts/collections may target "all clients." |
| `max_hunt_clients`         | int        | Hard cap on clients a single hunt may target. Must be > 0. |
| `require_approval_for`     | []string   | Operation categories gated by `internal/approval`. See [approval-flow.md](approval-flow.md). |
| `allowed_artifacts`        | []string   | The complete artifact allowlist. Nothing outside this list can be collected, hunted, or (by default) listed. |
| `allowed_profiles`         | []string   | The complete DFIR profile allowlist. See [dfir-profiles.md](dfir-profiles.md). |

### `audit`

| Field           | Type     | Notes |
|-----------------|----------|-------|
| `enabled`       | bool     | Should be `true` in every real deployment. |
| `path`          | string   | JSONL audit log path. Required when `enabled` is true. Written with `0600` permissions. |
| `redact_fields` | []string | Additive to the hard-coded default redaction list in `internal/audit/sanitize.go`; never a replacement for it. |

### `approval`

| Field         | Type   | Notes |
|---------------|--------|-------|
| `store_path`  | string | Path to the JSON file backing `internal/approval.FileStore`. **Optional; empty disables every approval-gated tool** — one of two settings (alongside `policy.mode: controlled`) that must both be set before the write pilot activates at all (see `mcpserver.writePilotEnabled`). Written to only by `Store.Consume` (from tool handlers) and by the separate `agentic-velociraptor-mcp approve` CLI subcommand run by a human operator — never by an MCP tool's `Create`/`Decide` path, since no MCP tool calls those. Must point at the exact same file the `approve` CLI's `--store` flag targets. |
| `ttl_seconds` | int    | How long an approved-but-unused request remains usable, measured from `Request.CreatedAt`. Must be > 0 when `store_path` is set. Must match (or be looser than) the `approve` CLI's `--ttl-seconds` for a request to be usable for as long as an operator expects. |

## The read API config file (`read_api_config_path`)

This is **not** this project's own config file — it's the
`api.config.yaml` a Velociraptor administrator generates for you via:

```sh
velociraptor --config server.config.yaml config api_client \
  --name "agentic-velociraptor-mcp-reader" \
  api.config.yaml
```

(See [velociraptor-permissions.md](velociraptor-permissions.md) for what
role/ACLs that identity should — and must not — have.)

`internal/velociraptor.LoadAPIConfig` reads it directly with the same
top-level YAML keys Velociraptor itself writes:

```yaml
ca_certificate: |
  -----BEGIN CERTIFICATE-----
  ...
  -----END CERTIFICATE-----
client_cert: |
  -----BEGIN CERTIFICATE-----
  ...
  -----END CERTIFICATE-----
client_private_key: |
  -----BEGIN RSA PRIVATE KEY-----
  ...
  -----END RSA PRIVATE KEY-----
api_connection_string: velociraptor.example.internal:8001
name: agentic-velociraptor-mcp-reader
# pinned_server_name: VelociraptorServer   # optional; see below
```

`ca_certificate`, `client_cert`, and `client_private_key` are secret
key material — treat this file exactly like an SSH private key:

- **File permissions are enforced, not just recommended.**
  `LoadAPIConfig` fails closed if the file is readable or writable by
  group or other on POSIX platforms (`chmod 0600 reader.api.config.yaml`
  or stricter). This check is skipped on Windows, where file mode bits
  aren't meaningful the same way.
- The file must exist and be a regular file (not a directory, not
  missing) — also enforced by `LoadAPIConfig` before any parsing is
  attempted.
- `ca_certificate`, `client_cert`, `client_private_key`, and
  `api_connection_string` are required; a file missing any of them is
  rejected with an error naming the missing field (never the file's
  actual contents).
- The TLS connection's expected server name defaults to
  `VelociraptorServer` (Velociraptor's own fixed pinned name for its
  self-signed server certificates) unless the file sets
  `pinned_server_name` to something else.
- `APIConfig`'s `String()`/`GoString()` methods are hard-coded to a
  redacted placeholder, so this struct can never leak key material
  through an accidental `%v`/`%+v` format or log line.

## Validation

`internal/config.Validate` performs structural checks only (see
`internal/config/validate.go`): required fields, positive limits, known
enum values, and the `allow_raw_vql == false` invariant. It does not
cross-check `allowed_profiles` against `profiles/*.yaml` contents or
verify Velociraptor-side ACLs — those are `internal/dfir.ValidateProfile`
and an operator responsibility respectively (see
[velociraptor-permissions.md](velociraptor-permissions.md)).

`read_api_config_path` is deliberately **not** validated for
loadability by `config.Validate` itself (an empty string is valid
config, meaning mock mode). That check happens once, eagerly, in
`cmd/agentic-velociraptor-mcp`'s `buildDeps`, via
`internal/velociraptor.NewGRPCClient` → `LoadAPIConfig`, when the field
is non-empty. A configured-but-broken path fails server startup
entirely (exit code 1, no stdio transport ever started) rather than
falling back to mock mode — see docs/lab-validation-plan.md's Phase 0.
