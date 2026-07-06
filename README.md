# agentic-velociraptor-mcp

A secure-by-design [MCP](https://modelcontextprotocol.io) server exposing
safe, auditable, policy-controlled [Velociraptor](https://docs.velociraptor.app/)
endpoint DFIR capabilities to MCP-compatible agents: endpoint visibility,
artifact collection, hunt management, evidence retrieval, IOC hunting,
and approved DFIR investigation profiles.

**Status: v0.1.0.** A real MCP stdio server is running, exposing 8
read-only tools: `velo_health_check`, `velo_search_clients`,
`velo_get_client_info`, `velo_list_artifact_names`,
`velo_get_artifact_details`, `velo_list_dfir_profiles`,
`velo_get_dfir_profile`, `velo_validate_dfir_profile`. The five
visibility tools make real Velociraptor gRPC calls (mTLS, via
`velociraptor.read_api_config_path`) — `Check`, `ListClients`,
`GetClient`, and `GetArtifacts`, never the generic VQL `Query` RPC — or
run in mock mode if that path is left unset. No tool can collect, hunt,
download, cancel, or run raw VQL. **Requires Go 1.25+ to build.** See
[PROJECT_STATE.md](PROJECT_STATE.md) for exactly what exists today and
[PROJECT_PLAN.md](PROJECT_PLAN.md) for the roadmap. Do not point this at
a production Velociraptor deployment.

## Why this exists

Velociraptor is a highly privileged DFIR backend: depending on
configured artifacts and ACLs, it can read arbitrary endpoint files,
enumerate processes, and run collections across an entire fleet. Handing
that surface to an LLM-driven agent without careful boundaries is
dangerous. This project's design starts from the assumption that:

- The MCP server is **not** the only security boundary — Velociraptor's
  own ACLs are the primary one.
- Nothing is collected, hunted, cancelled, or downloaded without going
  through an artifact/profile allowlist and (for anything that changes
  endpoint state or discloses evidence) a human approval step.
- Raw VQL is never exposed to an agent. Every capability maps to a
  reviewed Velociraptor artifact or DFIR profile.

See [docs/security-model.md](docs/security-model.md) for the full threat
model and [docs/architecture.md](docs/architecture.md) for how the
pieces fit together.

## Design principles

- Secure by design; read-only by default.
- stdio MCP transport first; HTTP/SSE only if explicitly requested later.
- Velociraptor gRPC API only (not the internal REST API), via mTLS
  (`api.config.yaml`).
- Separate, least-privilege read and write Velociraptor API identities —
  never `administrator`.
- No raw VQL, no generic remote-admin-shell tool, ever, in the stable
  core.
- Allowlisted artifacts, allowlisted DFIR profiles, typed tool schemas,
  strict input validation.
- Timeouts, row limits, byte limits, upload limits on every call.
- JSONL audit log with secret redaction and three exhaustive outcomes:
  `success`, `blocked`, `error`.
- Human approval required for every collection, hunt start/cancel, flow
  cancel, and evidence download.
- MCP-specific hardening: no credential passthrough (no tool accepts API
  keys/certificates/tokens as arguments), no arbitrary URL fetching,
  unimplemented tools are never registered, session IDs are never
  treated as authentication. See
  [docs/security-model.md](docs/security-model.md#mcp-specific-security-practices).

## Repository layout

```
cmd/agentic-velociraptor-mcp/   CLI entrypoint
internal/
  audit/        structured audit events, JSONL sink, secret redaction
  approval/     human-approval request/decision workflow
  config/       YAML config model + validation
  dfir/         DFIR profile model, registry, validation
  mcpserver/    MCP server + tool registration (8 of 24 registered so far)
  policy/       MCP-layer policy engine (allowlists, approval routing)
  validation/   strict input validation (client IDs, artifacts, IOCs, scope)
  velociraptor/ Velociraptor gRPC client (mTLS: health check, client search/detail, artifact catalog real; rest still placeholders)
  velociraptor/veloapi/  minimal, hand-scoped gRPC stubs (Check, ListClients, GetClient, GetArtifacts)
  vql/          allowlisted VQL template binding — no raw VQL execution
profiles/       DFIR profile definitions (YAML)
docs/           architecture, security model, config, tool, and ops docs
examples/       MCP Inspector and example client config snippets
tests/          integration-level tests (as they're added)
```

## Getting started

Requires **Go 1.25+** (the official MCP Go SDK dependency's minimum).

```sh
go build -o bin/agentic-velociraptor-mcp ./cmd/agentic-velociraptor-mcp
./bin/agentic-velociraptor-mcp --version
./bin/agentic-velociraptor-mcp --help

# Start the MCP stdio server (see examples/client-configs for a sample
# config.yaml; profiles/ ships 3 DFIR profile definitions):
./bin/agentic-velociraptor-mcp --config /path/to/config.yaml \
  --profiles-dir /path/to/agentic-velociraptor-mcp/profiles
```

Once running, the server speaks MCP over stdio and exposes exactly 8
tools — see [docs/tool-reference.md](docs/tool-reference.md) for the
current callable inventory and
[examples/inspector/README.md](examples/inspector/README.md) for how to
drive it with MCP Inspector. Track progress in
[PROJECT_STATE.md](PROJECT_STATE.md).

## Documentation

- [docs/architecture.md](docs/architecture.md)
- [docs/security-model.md](docs/security-model.md)
- [docs/approval-flow.md](docs/approval-flow.md)
- [docs/configuration.md](docs/configuration.md)
- [docs/tool-reference.md](docs/tool-reference.md)
- [docs/dfir-profiles.md](docs/dfir-profiles.md)
- [docs/velociraptor-permissions.md](docs/velociraptor-permissions.md)
- [docs/lab-validation-plan.md](docs/lab-validation-plan.md)
- [docs/production-deployment.md](docs/production-deployment.md)

## License

Apache-2.0. See [LICENSE](LICENSE).

## Acknowledgements / prior art

`socfortress/velociraptor-mcp-server` was reviewed as a feature-set
reference point during planning. No code from that project is used
here; this project's security model, tool boundaries, and implementation
are independent.
