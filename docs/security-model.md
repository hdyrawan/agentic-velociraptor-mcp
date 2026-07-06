# Security model

Status: draft. This document is the canonical statement of the threat
model and controls; keep it in sync as tools are implemented.

## Premise

Velociraptor is a highly privileged DFIR backend: it can read arbitrary
files, list processes, and (depending on artifact) execute commands on
every enrolled endpoint. An MCP server that exposes Velociraptor to an
LLM-driven agent is exposing a very large blast radius if it gets this
wrong. Two premises drive every design decision in this repository:

1. **The MCP server must not be the only security boundary.**
   Velociraptor-native controls (API client ACLs, artifact-level
   permissions) are the primary boundary. MCP-layer policy
   (`internal/policy`), approval (`internal/approval`), and validation
   (`internal/validation`) are a second, independent layer on top —
   not a replacement.
2. **Agents are not trusted, and endpoints are not trusted either.**
   An MCP client (agent) may be manipulated by prompt injection from
   data it reads elsewhere. Tool responses must not overstate what an
   endpoint reports: a compromised, disabled, or misconfigured
   Velociraptor client can return incomplete or actively misleading
   data, and tool responses should say so rather than imply
   ground truth.

## Trust boundaries

```
Untrusted: MCP client / agent input (tool arguments, free text)
   |
   v
Boundary 1: internal/validation  — syntactic allowlisting of every input
   |
   v
Boundary 2: internal/policy      — artifact/profile allowlists, mode
   |                                (read_only vs controlled), target-all
   |                                gating, raw VQL always denied
   v
Boundary 3: internal/approval    — human sign-off for write-capable ops,
   |                                tied to case ID + reason
   v
Boundary 4: internal/velociraptor — split read/write mTLS identities
   |
   v
Boundary 5 (primary): Velociraptor server-side ACLs on each API identity
   |
   v
Endpoints (untrusted data source — see "evidence honesty" below)
```

Boundaries 1–4 are this project's responsibility. Boundary 5 is operator
responsibility, documented in
[velociraptor-permissions.md](velociraptor-permissions.md), and is the
one that must hold even if 1–4 have a bug.

## Why no raw VQL, ever, in the stable core

VQL is a full query language with plugins that can read files, execute
processes, and write to disk depending on which Velociraptor plugins the
API identity's ACLs permit. A `run_vql` tool with any non-trivial ACL
set is a generic remote command execution primitive wrapped in MCP
framing. Restricting *which* VQL strings an agent can submit (e.g. via
a blocklist of dangerous plugins) is a losing pattern — blocklists over
an expressive query language are bypassable. The stable core therefore
never accepts caller-supplied VQL text at all: every operation maps to a
pre-authored, reviewed Velociraptor artifact or a fixed internal
template (`internal/vql`), invoked with bound parameters.

## Why not hidden/obscure artifacts as a control

"Security by artifact-name obscurity" (relying on an agent not knowing
an artifact exists) is not a control. Every artifact reachable by any
tool must be in the explicit `policy.allowed_artifacts` allowlist,
reviewed the same way regardless of how likely an agent is to guess its
name.

## Approval model

See [approval-flow.md](approval-flow.md) for the full workflow. Summary
invariant: any operation that changes endpoint state (collection start,
hunt start/cancel, flow cancel) or discloses raw evidence bytes (upload
download) must have a corresponding, matching, single-use
`approval.Decision` before `internal/velociraptor`'s write-identity
client is called. The requesting agent/session must not be able to
self-approve.

## Least privilege for the MCP API identities

Neither the read nor the write API identity may hold:

- `administrator`
- `ARTIFACT_WRITER`
- `SERVER_ARTIFACT_WRITER`
- `EXECVE`
- `FILESYSTEM_WRITE`
- `SERVER_ADMIN`

See [velociraptor-permissions.md](velociraptor-permissions.md) for the
full recommended ACL set per identity and the reasoning per excluded
permission.

## Secrets handling

API config files (`read_api_config_path`, `write_api_config_path`)
contain client private keys and certificates and must be treated as
secrets: filesystem permissions restricting access to the service
account only, never logged, never included in audit events or error
messages. `internal/audit/sanitize.go` is the single choke point
responsible for redacting `client_private_key`, `client_cert`,
`ca_certificate`, `api_key`, `approval_token`, `password`, and `secret`
fields (configurable, additive, via `audit.redact_fields`) from every
audit event before it is written.

As of v0.1.0-alpha.2, `read_api_config_path` is actually loaded and
used (`internal/velociraptor.LoadAPIConfig` /
`internal/velociraptor/grpcclient.go`). Several layers enforce the
"never logged" requirement for this specific file, independent of the
audit sanitizer above (which operates on structured `audit.Event`
fields, not on this struct):

- `LoadAPIConfig` refuses to read a file that is group/other
  readable/writable on POSIX platforms (must be `chmod 0600` or
  stricter) — a defense against the file being left accidentally
  world-readable, not just a logging concern.
- `APIConfig.String()` and `APIConfig.GoString()` are hard-coded to a
  fixed redacted string, so passing an `APIConfig` value to `fmt`,
  `log`, or an `%v`/`%+v`-formatted error can never print key material,
  regardless of who writes that code in the future.
- `internal/velociraptor.sanitizeTLSError` strips anything that looks
  like a PEM block (`-----BEGIN`) out of gRPC/TLS error text before it
  reaches any visibility tool's output or the audit log, as a
  belt-and-suspenders measure — standard library TLS/x509 errors don't
  actually embed key bytes, but an error message is exactly the kind of
  string that tends to get pasted into a ticket or chat without a
  second thought. As of v0.1.0 this applies uniformly to all five
  visibility tools (`velo_health_check`, `velo_search_clients`,
  `velo_get_client_info`, `velo_list_artifact_names`,
  `velo_get_artifact_details`), not just the health check: every
  `grpcClient` method wraps its RPC error through the same
  `sanitizeTLSError` call before returning it.
- Error messages about a bad API config file **do** include the file
  *path* (an operator-supplied, non-sensitive location string) — only
  the file's *contents* are treated as secret.

## Evidence honesty

Tool response text must not claim more certainty than the underlying
Velociraptor data supports. Concretely:

- Result-set responses must state when results were truncated by
  `max_rows`/`max_result_bytes`, not silently present a partial view as
  complete.
- Absence of a finding (e.g. "no persistence artifacts found") must be
  phrased as "the endpoint did not report X in the collected data,"
  not "the endpoint has no X" — the client agent may be offline,
  compromised, or the collection may have failed partially.
- Flow/hunt status responses should surface `error`/`cancelled` states
  plainly rather than only reporting rows returned.
- `velo_health_check` (v0.1.0-alpha.2) is the first concrete example of
  this principle: a real-mode health check that fails to reach
  Velociraptor returns `velociraptor_connected: false` with a
  `status: "error"` and an explanatory message as a normal, successful
  tool result — not a Go-level tool error — because "Velociraptor is
  unreachable" is data the caller asked for, not a failure of the
  health-check tool itself. A mock-mode response never claims
  `velociraptor_connected: true`.
- All four v0.1.0 visibility tools (`velo_search_clients`,
  `velo_get_client_info`, `velo_list_artifact_names`,
  `velo_get_artifact_details`) follow the same pattern: every response
  carries an explicit `mode` field (`"mock"` or `"real"`), a
  connectivity or lookup failure is reported as an empty/`nil` result
  plus a `message` field rather than a Go-level tool error, and mock
  mode never returns a populated result list. Only *input validation*
  failures (a malformed client ID, artifact name, or search query) and
  policy-allowlist blocks are reported as Go-level tool errors, since
  those are static defects in the request itself, not information about
  Velociraptor's state.
- **v0.2.0** formalized this pattern with a shared `internal/response`
  envelope (a `status` field: `"success"` / `"empty"` / `"not_found"` /
  `"error"`, embedded into `SearchClientsOutput`, `GetClientInfoOutput`,
  `ListArtifactNamesOutput`, and `GetArtifactDetailsOutput`) so callers
  can branch on `status` instead of inferring outcome from `mode` plus
  the presence/absence of a data field. This is additive to the wire
  shape (a new top-level `status` key) and did not change any existing
  field's meaning. `velo_health_check`'s pre-existing `status` field
  (`"ok"`/`"error"`, from v0.1.0-alpha.2, predating this envelope) is
  deliberately left untouched to avoid a breaking wire-value change. As
  part of the same pass, `velo_get_client_info` and
  `velo_get_artifact_details` gained a distinct `"not_found"` status for
  a genuine "no such client"/"no such artifact" lookup (via
  `velociraptor.ErrClientNotFound` / the new `velociraptor.ErrArtifactNotFound`
  sentinel), which was previously indistinguishable from any other
  real-mode connectivity/RPC failure.
- **v0.3.0** extends the same response envelope to the three read-only
  workflow helpers (`velo_plan_dfir_triage`,
  `velo_compare_dfir_profiles`, `velo_find_profiles_by_artifact`). These
  helpers report empty/no-match and not-found profile/artifact coverage
  as structured `status` values, but they are planning metadata only:
  they do not claim that any endpoint was examined and they do not
  execute collections or hunts.
- `velo_get_client_info` and `velo_search_clients` return exactly what
  Velociraptor's `ApiClient` record reports for a given endpoint
  (hostname, OS, last-seen time, labels, ...) with no independent
  verification — this is client-reported telemetry, not a
  server-attested fact. A compromised or spoofed client could report
  false values for these fields; nothing in this milestone treats them
  as trustworthy for anything beyond triage/selection purposes.
- **v0.5.0** backfills the three read-only flow/result handlers
  (`velo_list_flows`, `velo_get_flow_status`, `velo_get_flow_results`)
  onto the same response envelope. They validate client/flow IDs before
  any read-client call, report `not_found` separately from operational
  errors when the read client returns the sentinel errors, and make
  truncation explicit (`truncated`, `next_cursor`, row/byte counts)
  rather than presenting partial results as complete. The current real
  gRPC backend does not yet implement flow RPCs, so real-mode flow calls
  return a structured `error` rather than fabricated data until a
  reviewed backend is added.
- **v0.4.0** extends the same envelope to the six new write-capable
  tools, adding a distinction the earlier read-only tools didn't need:
  an unresolved `approval_reference` is `status: "not_found"` (a normal,
  honest "that reference doesn't exist yet" answer, not a crash), while
  a resolved-but-not-usable reference (undecided, denied, expired,
  already consumed, or fingerprint-mismatched) is `status: "error"` with
  a specific message — both are audited `blocked`, distinguishing "we
  don't know about this" from "we know about this and it's not
  authorized." A real write-RPC failure (once real RPC wiring exists) is
  also `status: "error"`, audited `error` instead of `blocked` — see
  `internal/mcpserver/server.go`'s `verifyAndConsumeApproval` and
  `docs/approval-flow.md`.

## Dependency surface: no upstream Velociraptor server module

The real gRPC client added in v0.1.0-alpha.2 does not import
`github.com/Velocidex/velociraptor` (the upstream server's own Go
module). That module is an entire DFIR server — CGO dependencies,
YARA/Capstone bindings, a datastore layer, VQL execution engine, and
dozens of unrelated gRPC services — none of which this project needs or
wants in its dependency tree or binary. Pulling it in would mean this
project's supply-chain surface includes code paths (arbitrary VQL
execution, artifact writing, server administration) that everything
else in this document argues against exposing at all.

Instead, `internal/velociraptor/veloapi/` is a small, hand-authored set
of `.proto` files defining only the RPCs this project actually calls —
as of v0.1.0: `API.Check`, `API.ListClients`, `API.GetClient`, and
`API.GetArtifacts` — with field names, field numbers, and
package/service names copied from upstream's own `api/proto/health.proto`,
`api/proto/clients.proto`, `api/proto/artifacts.proto`,
`artifacts/proto/artifact.proto`, and `api/proto/api.proto` so the
generated client is wire-compatible with a real Velociraptor server.
It's compiled with `buf` + `protoc-gen-go`/`protoc-gen-go-grpc` (see the
comment at the top of each `.proto` file for the regeneration command)
into ordinary, auditable generated Go code — not hand-rolled wire
encoding. Every message here also deliberately declares only the fields
its tool needs: `visibility.proto`'s `Artifact` message, for example, has
no field for `ArtifactSource` (upstream field 4), which is where an
artifact's VQL query body lives — a field this project's generated Go
struct has no way to decode into, so it can never reach a tool response
even by accident, regardless of what the server sends. Extending this
project to a new RPC (e.g. a real `Query` call, if that's ever justified
for a specific, reviewed reason) means adding the specific
message/service definitions needed for that one call, not vendoring the
upstream module.

Notably, none of the four RPCs above is the generic `Query` RPC
(streaming free-form VQL): `ListClients`, `GetClient`, and
`GetArtifacts` are purpose-built, narrow RPCs that return exactly the
structured data `velo_search_clients`, `velo_get_client_info`,
`velo_list_artifact_names`, and `velo_get_artifact_details` need, with
every caller-supplied value (search query, client ID, artifact name)
bound as a plain protobuf field — never a VQL string, bound parameter,
or any other query-language construct. This sidesteps the raw-VQL
question entirely for this milestone rather than relying on
`internal/vql`'s template/parameter-binding mechanism, which remains
reserved for the one case (`velo_hunt_ioc_with_approval`, unscheduled)
that has no purpose-built RPC equivalent.

## Out of scope for this MCP server (by design)

- Generic remote command execution / shell (no `run_vql`, no arbitrary
  process-execution tool).
- Acting as the sole authorization boundary — operators must still
  configure least-privilege Velociraptor ACLs.
- HTTP/SSE transports before stdio is stable and explicitly requested;
  see PROJECT_PLAN.md.

## MCP-specific security practices

These follow the Model Context Protocol's own security guidance, layered
on top of the Velociraptor-specific controls above. As of
v0.1.0-alpha.1, `internal/mcpserver` implements the transport and
tool-minimization items below; the rest (approval binding, HTTP-mode
authorization) are forward-looking constraints on future milestones and
are recorded here so they aren't relitigated when v0.2.0/HTTP-mode work
starts.

### Transport: stdio only, for now

The server only supports the stdio transport
(`internal/mcpserver.Server.Run` calls `mcp.StdioTransport{}` exclusively).
There is no HTTP/SSE/streamable HTTP listener, so there is no network
attack surface to reason about yet. See
[PROJECT_PLAN.md](../PROJECT_PLAN.md)'s "MCP Security Best-Practice
Integration" section for the authorization/binding requirements any
future HTTP transport must satisfy before it ships, and note in
particular: **session IDs must never be treated as authentication or
authorization**, even once a session-oriented transport exists.

### No credential passthrough

No MCP tool accepts Velociraptor API keys, client certificates, private
keys, bearer tokens, or raw `api.config.yaml` contents as an argument.
Velociraptor credentials are exclusively server-side secrets, loaded
once at startup from `velociraptor.read_api_config_path` /
`write_api_config_path` (see [configuration.md](configuration.md)). An
MCP client can request *that an operation happen*; it can never supply
*which credential* performs it. This also means a malicious or
compromised MCP client cannot exfiltrate Velociraptor credentials
through a tool call, because no code path ever accepts them as input in
the first place.

### No arbitrary URL fetching

No tool in the stable core fetches a caller-supplied URL. Every tool's
network behavior is bounded to a single, statically configured
Velociraptor server per (`read_api_config_path` /
`write_api_config_path`); nothing in a tool's arguments can redirect a
call elsewhere. This forecloses the common MCP-server SSRF pattern
where an agent is tricked into making the server fetch an
attacker-controlled URL.

### Unimplemented tools are never registered

`internal/mcpserver` only calls `mcp.AddTool` for tools that are fully
implemented and reviewed for the current milestone — visibility,
profile, workflow, and (as of v0.4.0) the six controlled collection
pilot tools. Hunt-execution, hunt read/preview, and IOC execution tools
exist only as `ToolSpec` metadata in `tools_hunts.go` and `tools_ioc.go`
(used for `docs/tool-reference.md` generation); `velo_list_flows`,
`velo_get_flow_status`, and `velo_get_flow_results` exist as metadata in
`tools_flows.go` alongside the now-implemented upload/download tools.
None of these are reachable by any MCP client — confirmed in
`internal/mcpserver/server_test.go`'s exact-tool-inventory test. A
planned tool becomes callable only when its milestone lands with real
validation, policy, and (where required) approval wiring, not when its
metadata is added.

### Confused-deputy mitigation is implemented, not just designed (v0.4.0)

`internal/approval.RequestFingerprint` hashes the security-relevant
targeting fields of a `Request` (operation, case ID, client ID,
artifact, parameters, profile, hunt ID, flow ID, upload name).
`mcpserver.verifyAndConsumeApproval` (`internal/mcpserver/server.go`)
recomputes this fingerprint for the operation a tool call is about to
perform and confirms it matches the fingerprint of the resolved,
approved `Request` before calling Velociraptor — a mismatch (different
artifact, different client, different parameters) is rejected, not
silently substituted. An approval is a grant for one exact, hashed
payload, consumed exactly once (`Store.Consume`, called *before* the
Velociraptor call so a failed attempt still burns the approval) — never
a standing grant of "this operation category is now approved."

The deeper mitigation is architectural: **no MCP tool can call
`Store.Create` or `Store.Decide`.** Those are only reachable from the
`agentic-velociraptor-mcp approve` CLI subcommand, run directly by a
human operator, never through the MCP stdio transport. An MCP client
(including an LLM driving tool calls) can request that a write-capable
operation happen, by supplying an `approval_reference`; it can never
make that reference valid. See [approval-flow.md](approval-flow.md) for
the full workflow, including known limitations (single-analyst pilot,
no cross-process file locking, no real Velociraptor RPC wiring yet for
the underlying collection/cancel/upload calls themselves).

### Tool and scope minimization is intentional

The stable core deliberately exposes 27 narrow tools rather than a small
number of broad, parameterizable ones (and no raw-VQL escape hatch at
all). As of v0.4.0 (rebased onto v0.5.0), 20 of those 27 are registered:
14 read-only tools plus six approval-gated write tools, each of which is
still scoped to a single client per call, still requires explicit
operator configuration (`policy.mode: controlled` and
`approval.store_path`, plus `velociraptor.download_dir` for the download
tool) before it does anything beyond report itself disabled, and none of
which is a hunt or raw-VQL tool. Minimizing the callable surface at
every point in time — not just in the final v1.0.0 design — reduces both
the attack surface and the chance an agent misuses a capability it
didn't need for the task at hand. When a future HTTP/remote transport is
added, this same principle should extend to authorization scopes (see
PROJECT_PLAN.md's scope list, e.g. `velo:read`, `velo:profiles:read`,
`velo:collect`) rather than an all-or-nothing API token.
