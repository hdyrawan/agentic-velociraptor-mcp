# Architecture

Status: draft, describes the target architecture for v1.0.0. Only the
package skeleton exists as of v0.0.x; see PROJECT_STATE.md for what is
actually implemented today.

## Component overview

```
                         +--------------------------+
  MCP client (agent)  -->|  internal/mcpserver      |
  (stdio transport)      |  - tool handlers (24)    |
                         |  - request validation    |
                         +------------+-------------+
                                      |
              +-----------------------+------------------------+
              |                       |                        |
   +----------v---------+  +---------v----------+   +----------v---------+
   | internal/policy    |  | internal/approval   |   | internal/audit    |
   | (MCP-layer policy)  |  | (human approval     |   | (JSONL audit log, |
   |                     |  |  workflow)           |   |  secret redaction)|
   +----------+---------+  +---------+----------+   +----------+---------+
              |                       |                        |
              +-----------+-----------+------------------------+
                          |
              +-----------v-----------+       +--------------------+
              | internal/dfir         |------>| profiles/*.yaml     |
              | (DFIR profile registry|       | (reviewed, versioned|
              |  + validation)        |       |  artifact bundles)  |
              +-----------+-----------+       +--------------------+
                          |
              +-----------v-----------+       +--------------------+
              | internal/vql          |       | internal/validation |
              | (allowlisted template |<----->| (client ID/artifact/ |
              |  binding, no raw VQL) |       |  IOC/scope checks)   |
              +-----------+-----------+       +--------------------+
                          |
              +-----------v-----------+
              | internal/velociraptor |
              | (gRPC client, split   |
              |  read vs write API    |
              |  identity)            |
              +-----------+-----------+
                          |
              mTLS (api.config.yaml)
                          |
              +-----------v-----------+
              |  Velociraptor server   |
              |  (gRPC API)            |
              +------------------------+
```

## Request lifecycle (write-capable tool example)

1. MCP client calls a tool, e.g. `velo_collect_artifact_with_approval`.
2. `internal/mcpserver` validates the typed request shape.
3. `internal/validation` checks client ID / artifact name syntax.
4. `internal/policy` checks the artifact against the allowlist and
   confirms the operation category requires (or is even eligible for)
   approval; read-only mode short-circuits to a `blocked` audit event.
5. `internal/approval` is consulted for an existing, matching, unused
   approval `Decision`. If absent, the tool returns instructions for
   requesting approval rather than performing any Velociraptor call.
6. Only once approved: `internal/velociraptor` performs the call using
   the **write** API identity, with parameters bound via
   `internal/vql`/`internal/dfir` (never string-concatenated VQL).
7. `internal/audit` writes exactly one `Event` recording the outcome
   (`success`, `blocked`, or `error`), with secrets redacted.

Read-only tools skip steps 5–6's approval gate but still pass through
validation, policy (allowlist checks), and audit.

## Two Velociraptor API identities

The server holds two independent Velociraptor gRPC client connections,
each authenticated via its own mTLS `api.config.yaml`:

- **Read identity** (`velociraptor.read_api_config_path`): used for every
  visibility/read tool. Should hold the minimum ACLs to search clients,
  read artifact metadata, and read flow/hunt results.
- **Write identity** (`velociraptor.write_api_config_path`): used only
  after approval, for collection, hunt start/cancel, and upload
  download. Still least-privilege — see
  [velociraptor-permissions.md](velociraptor-permissions.md) for the
  specific ACL set and the permissions it must never hold.

Using two identities means a compromised or bugged read-path tool cannot
mutate endpoint state even if MCP-layer policy checks were bypassed:
Velociraptor itself would reject the write attempt.

## Why gRPC, not the internal REST API

Velociraptor's internal REST API is intended for its own GUI and is not
a stable, documented integration surface. The gRPC API (`api.proto`) is
the documented, versioned integration point for external tools and is
what this project integrates against exclusively.

## Why no raw VQL tool

See [security-model.md](security-model.md) for the full rationale. In
short: a generic `run_vql` tool is equivalent to a remote administration
shell against every enrolled endpoint, and no amount of MCP-layer policy
wrapping fully mitigates that. The stable core instead exposes narrow,
typed tools backed by allowlisted artifacts and profiles.
