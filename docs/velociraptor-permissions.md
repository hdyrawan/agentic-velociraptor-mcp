# Velociraptor permissions

Status: draft operator guidance. This is the primary security boundary
(see [security-model.md](security-model.md)) — get this right
independent of anything the MCP-layer code does.

## Two separate API client identities

Create two separate Velociraptor API clients (`velociraptor config
api_client` or via the GUI's "Server Users" / API client management),
each with its own `api.config.yaml` and its own dedicated Velociraptor
user/role:

- **Reader** → `velociraptor.read_api_config_path`
- **Investigator (write)** → `velociraptor.write_api_config_path`

Do not reuse one identity for both. Do not create either as
`administrator`.

## Never grant these permissions to either identity

| Permission | Why it's excluded |
|-------------|--------------------|
| `administrator` | Full server control; violates least privilege by definition. |
| `ARTIFACT_WRITER` | Lets the identity define/modify client-side artifacts — equivalent to being able to author new remote-execution capability. |
| `SERVER_ARTIFACT_WRITER` | Same, for server-side artifacts; also affects the Velociraptor server process itself. |
| `EXECVE` | Direct process execution on endpoints. Nothing this server does needs this — DFIR profiles are read/collection oriented. |
| `FILESYSTEM_WRITE` | Write access to endpoint filesystems. Nothing this server does needs this. |
| `SERVER_ADMIN` | Server-level administration (users, orgs, config). |

`internal/policy/rules.go`'s `DangerousVelociraptorPermissions` mirrors
this list for documentation/checklist purposes.

## Recommended reader permission set

Enough to search clients, read client metadata, browse the artifact
catalog, and read existing flow/hunt results:

- `READ_RESULTS`
- `LABEL_CLIENT` (only if the deployment uses label-based search; can be
  omitted if not needed)

Confirm current Velociraptor documentation for the exact ACL names
available in the deployed server version — ACL names have evolved
across Velociraptor releases; treat this list as a starting point to
verify, not a copy-paste final answer.

### `velo_health_check` needs no ACL beyond a valid client certificate

As of v0.1.0-alpha.2, `velo_health_check` calls Velociraptor's `Check`
gRPC RPC (`api/proto/api.proto`'s `rpc Check(HealthCheckRequest) returns
(HealthCheckResponse)`). Upstream's own server-side handler
(`api/health.go`) ignores the request entirely and unconditionally
returns `SERVING` — it does not check the caller's role or any specific
permission. In practice this means: if your reader identity's mTLS
certificate is valid and the server accepts the connection at all,
`Check` will succeed regardless of which (if any) of the roles/ACLs
above it also holds. This is expected and matches the intent of a basic
health/liveness check; it is not a reason to grant the reader identity
anything beyond the minimal set above, since every *other* tool this
project will add still needs the ACLs it actually uses.

## Recommended investigator (write) permission set

Everything in Reader, plus enough to launch collections/hunts against
already-enrolled clients and cancel them:

- `COLLECT_CLIENT` (scoped, if the server version supports scoping to an
  artifact allowlist at the ACL level — use it in addition to, not
  instead of, this project's `policy.allowed_artifacts`)
- `COLLECT_SERVER` only if server-side hunts require it in the deployed
  version, otherwise omit
- `READ_RESULTS`

Explicitly excluded even from the write identity: everything in the
"never grant" table above. Collection and hunting do not require
`EXECVE` or `FILESYSTEM_WRITE` — those are file/process artifacts'
concern on the endpoint side, gated by which artifacts are allowlisted,
not by the API identity's own permissions.

## Defense in depth: even a fully-scoped identity should be paired with a restrictive artifact allowlist

Velociraptor ACLs like `COLLECT_CLIENT` are typically not artifact-scoped
in older server versions (verify against the deployed version). Assume
they are not, and rely on `policy.allowed_artifacts` /
`policy.allowed_profiles` as the actual artifact-level allowlist. The
Velociraptor-side ACL then bounds the *category* of action (collect vs.
not), while this project's config bounds *which* artifacts.

## Certificate/config hygiene

- `read_api_config_path` / `write_api_config_path` files contain private
  keys. Store with `0600` permissions, owned by the service account
  running this server, outside version control. **As of
  v0.1.0-alpha.2, `0600` is enforced, not just recommended** — a stricter
  file mode (readable/writable by group or other) makes
  `internal/velociraptor.LoadAPIConfig` refuse to read the file at all,
  on POSIX platforms.
- Rotate API client certificates on the same cadence as other
  high-privilege service credentials in your environment.
- Never paste the contents of an `api.config.yaml` into a chat, ticket,
  or log line, including for debugging — see
  [security-model.md](security-model.md)'s secrets handling section.
- The TLS connection verifies the server's certificate against the name
  `pinned_server_name` in the api.config.yaml, or `VelociraptorServer`
  (Velociraptor's own default) if that field is absent. If your
  deployment issues server certificates under a different pinned name,
  set `pinned_server_name` explicitly in the generated api.config.yaml —
  don't disable server name verification to work around a mismatch.

## Pre-flight checklist before pointing this server at a real Velociraptor deployment

- [ ] Reader and Investigator are distinct Velociraptor users/identities.
- [ ] Neither holds any permission in the "never grant" table.
- [ ] `policy.allowed_artifacts` only lists artifacts you have reviewed.
- [ ] `policy.allow_target_all` is `false` unless you have a specific,
      reviewed reason to allow all-client targeting.
- [ ] `audit.enabled` is `true` and `audit.path` is on a filesystem with
      retention/backup appropriate for investigation records.
- [ ] `policy.mode` is `read_only` until you are ready to exercise the
      approval workflow end to end in a lab (see
      [lab-validation-plan.md](lab-validation-plan.md)).


## v0.8.0 backend wiring status

v0.8.0 is a backend-wiring review milestone that preserves the v0.7.0 28-tool MCP inventory. The hand-authored `internal/velociraptor/veloapi` mirror currently exposes only `Check`, `ListClients`, `GetClient`, and `GetArtifacts`; it does not include reviewed typed RPC bindings for flow enumeration/results, collection execution, flow cancel, uploads, hunt execution/cancel, hunt results, or IOC hunt execution. Implementing those by exposing a generic VQL query path would violate the stable-core raw-VQL rule, so they remain scaffolded with structured errors.

| Group | v0.8.0 status |
|---|---|
| Visibility (`health`, client search/info, artifact list/details) | Real gRPC already implemented and unchanged. |
| Flow list/status/results | Handler contracts, validation, limits, pagination, audit unchanged; real gRPC remains scaffolded (`backend_not_implemented`/`error`, no panic). |
| Collection start / DFIR profile collection / flow cancel | Approval/policy/input/allowlist gates unchanged; backend capability is now checked before consuming approval; real gRPC remains scaffolded. |
| Flow uploads list/metadata/download | Read handlers and download file controls unchanged; download backend capability is now checked before consuming approval; real gRPC upload RPCs remain scaffolded. |
| Hunts list/status/results/preview | Handler contracts, limits, target_all/max-client policy unchanged; real gRPC remains scaffolded. |
| Approved hunt start/cancel and IOC hunt | Approval fingerprint/scope/template gates unchanged; backend capability is now checked before consuming approval; real gRPC hunt RPCs remain scaffolded. |

Live-lab validation remains pending for every scaffolded operation above. Required follow-up: add reviewed typed protobuf bindings for the specific Velociraptor RPCs, prove least-privilege read/write API permissions in a disposable lab, and keep `max_rows`, `max_result_bytes`, `max_upload_bytes`, `max_hunt_clients`, `target_all`, cursor, audit, and no-raw-VQL invariants under test.
