# agentic-velociraptor-mcp

A secure-by-design [MCP](https://modelcontextprotocol.io) server that exposes
safe, auditable, policy-controlled [Velociraptor](https://docs.velociraptor.app/)
endpoint DFIR capabilities to MCP-compatible agents — endpoint visibility,
collection flows/results, and reviewed DFIR investigation profiles today,
with approved artifact collection, hunt management, evidence retrieval,
and IOC hunting on the roadmap.

**Status: v0.5.0**, 14 callable read-only MCP tools. Nothing collects,
hunts, downloads, cancels, mutates a client, or runs raw VQL yet. See
[PROJECT_STATE.md](PROJECT_STATE.md) for the exact current tool
inventory and [PROJECT_PLAN.md](PROJECT_PLAN.md) for the roadmap. Do
not point this at a production Velociraptor deployment.

## Contents

- [Why this exists](#why-this-exists)
- [Quick start](#quick-start)
  - [Option A: Docker](#option-a-docker)
  - [Option B: Build from source](#option-b-build-from-source)
- [Configure the Velociraptor connection](#configure-the-velociraptor-connection)
- [Connect an MCP client](#connect-an-mcp-client)
  - [Claude Desktop](#claude-desktop)
  - [Claude Code](#claude-code)
  - [Hermes](#hermes)
  - [OpenCode](#opencode)
  - [Any other MCP client / MCP Inspector](#any-other-mcp-client--mcp-inspector)
- [Design principles](#design-principles)
- [Repository layout](#repository-layout)
- [Documentation](#documentation)
- [License](#license)

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

## Quick start

You need a running instance of this server that an MCP client can
launch — either a container image or a locally built binary. Both are
configured the same way: a YAML config file (`--config`) plus a
directory of DFIR profile definitions (`--profiles-dir`).

### Option A: Docker

```sh
git clone https://github.com/hdyrawan/agentic-velociraptor-mcp.git
cd agentic-velociraptor-mcp
docker build -t agentic-velociraptor-mcp:latest .
```

The image runs as a non-root user, ships no shell, and only ever
listens on stdio (no port is exposed). It needs a config file mounted
read-only, and — if `audit.enabled: true` in that config (the default)
— a writable directory for the audit log:

```sh
mkdir -p ./audit
docker run --rm -i \
  -v "$(pwd)/examples/client-configs/config.read-only.example.yaml:/etc/agentic-velociraptor-mcp/config.yaml:ro" \
  -v "$(pwd)/audit:/var/log/agentic-velociraptor-mcp" \
  agentic-velociraptor-mcp:latest
```

That example config runs entirely in mock mode (no real Velociraptor
connection) so it works with zero further setup — useful for confirming
the container itself is healthy before wiring in a real
`read_api_config_path`. The process reads/writes stdio and exits on
EOF; an MCP client is what normally keeps it alive and talks to it (see
[Connect an MCP client](#connect-an-mcp-client) below). To point it at a
real Velociraptor server, mount your own `config.yaml` and the
`api.config.yaml` file(s) it references (see
[Configure the Velociraptor connection](#configure-the-velociraptor-connection)):

```sh
docker run --rm -i \
  -v "/etc/agentic-velociraptor-mcp/config.yaml:/etc/agentic-velociraptor-mcp/config.yaml:ro" \
  -v "/etc/agentic-velociraptor-mcp/secrets:/etc/agentic-velociraptor-mcp/secrets:ro" \
  -v "/var/log/agentic-velociraptor-mcp:/var/log/agentic-velociraptor-mcp" \
  agentic-velociraptor-mcp:latest
```

### Option B: Build from source

Requires **Go 1.25+** (the official MCP Go SDK dependency's minimum).

```sh
git clone https://github.com/hdyrawan/agentic-velociraptor-mcp.git
cd agentic-velociraptor-mcp
go build -o bin/agentic-velociraptor-mcp ./cmd/agentic-velociraptor-mcp

./bin/agentic-velociraptor-mcp --version
./bin/agentic-velociraptor-mcp --help

# Runs entirely in mock mode out of the box:
./bin/agentic-velociraptor-mcp \
  --config examples/client-configs/config.read-only.example.yaml \
  --profiles-dir profiles
```

Either way, the server speaks MCP over stdio and exposes exactly 14
read-only tools — see [docs/tool-reference.md](docs/tool-reference.md)
for the current callable inventory.

## Configure the Velociraptor connection

By default (`velociraptor.read_api_config_path` empty, as in
`examples/client-configs/config.read-only.example.yaml`) the server runs
in **mock mode**: it never calls a real Velociraptor server, and every
tool response says so explicitly. To connect to a real Velociraptor
deployment:

1. Have a Velociraptor administrator generate a **least-privilege
   reader** API client identity (never `administrator`) — see
   [docs/velociraptor-permissions.md](docs/velociraptor-permissions.md).
2. Save the resulting `api.config.yaml` outside version control and
   `chmod 0600` it.
3. Point `velociraptor.read_api_config_path` at it in your own copy of
   `config.yaml` (see
   [examples/client-configs](examples/client-configs) for the full
   shape and [docs/configuration.md](docs/configuration.md) for every
   field).

A misconfigured-but-set path fails the server closed at startup rather
than silently falling back to mock mode.

## Connect an MCP client

Every client below ultimately needs the same two things: a command to
launch the server (the binary path, or `docker run ...`) and its
arguments (`--config`, `--profiles-dir`). Substitute your own paths for
the placeholders shown.

### Claude Desktop

Edit the config file at:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`
- Linux: `~/.config/Claude/claude_desktop_config.json`

**Local binary:**

```json
{
  "mcpServers": {
    "velociraptor": {
      "command": "/path/to/agentic-velociraptor-mcp/bin/agentic-velociraptor-mcp",
      "args": [
        "--config", "/path/to/config.yaml",
        "--profiles-dir", "/path/to/agentic-velociraptor-mcp/profiles"
      ]
    }
  }
}
```

**Docker:**

```json
{
  "mcpServers": {
    "velociraptor": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-v", "/etc/agentic-velociraptor-mcp/config.yaml:/etc/agentic-velociraptor-mcp/config.yaml:ro",
        "-v", "/var/log/agentic-velociraptor-mcp:/var/log/agentic-velociraptor-mcp",
        "agentic-velociraptor-mcp:latest"
      ]
    }
  }
}
```

Restart Claude Desktop after saving. See Anthropic's
[MCP quickstart](https://modelcontextprotocol.io/quickstart/user) if the
config file doesn't exist yet.

### Claude Code

From the repository (or any project) root:

```sh
claude mcp add velociraptor -- /path/to/agentic-velociraptor-mcp/bin/agentic-velociraptor-mcp \
  --config /path/to/config.yaml \
  --profiles-dir /path/to/agentic-velociraptor-mcp/profiles
```

Or add it directly to a project's `.mcp.json` (equivalent JSON shape to
the Claude Desktop example above). Run `claude mcp list` to confirm it's
registered, then check available tools with `/mcp` inside a Claude Code
session.

### Hermes

Most MCP-compatible agent hosts, including Hermes, use the same
`mcpServers` JSON shape as Claude Desktop above (a `command` plus
`args` array for a stdio server). Add an entry named `velociraptor`
pointing at either the local binary or the Docker invocation shown
above in whichever config file your Hermes installation reads MCP
server definitions from. Consult Hermes's own documentation for that
file's exact location — this project doesn't assume a specific path
since that detail is client-specific and outside this repository's
control.

### OpenCode

[OpenCode](https://opencode.ai) reads MCP server definitions from its
own config (`opencode.json`, or the `mcp` section of your OpenCode
config — see OpenCode's docs for the exact key names in your installed
version). The shape is the same local-vs-Docker choice as above:

```json
{
  "mcp": {
    "velociraptor": {
      "type": "local",
      "command": [
        "/path/to/agentic-velociraptor-mcp/bin/agentic-velociraptor-mcp",
        "--config", "/path/to/config.yaml",
        "--profiles-dir", "/path/to/agentic-velociraptor-mcp/profiles"
      ]
    }
  }
}
```

If your OpenCode version expects a different schema (e.g. a flat
`command`/`args` pair like the Claude examples instead of a single
`command` array), adjust to match — check `opencode --help` or
OpenCode's MCP documentation for what your installed version expects.

### Any other MCP client / MCP Inspector

Any MCP client that can launch a local stdio server works the same way:
give it the binary (or `docker run ...`) command and the two flags
above. For manual testing without any agent host at all, use
[MCP Inspector](https://github.com/modelcontextprotocol/inspector) — see
[examples/inspector/README.md](examples/inspector/README.md) for a
worked example against this server specifically.

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
  mcpserver/    MCP server + tool registration (14 registered so far)
  policy/       MCP-layer policy engine (allowlists, approval routing)
  validation/   strict input validation (client IDs, artifacts, IOCs, scope)
  velociraptor/ Velociraptor gRPC client (mTLS: health check, client search/detail, artifact catalog, flow/result reads; rest still placeholders)
  velociraptor/veloapi/  minimal, hand-scoped gRPC stubs (Check, ListClients, GetClient, GetArtifacts)
  vql/          allowlisted VQL template binding — no raw VQL execution
profiles/       DFIR profile definitions (YAML)
docs/           architecture, security model, config, tool, and ops docs
examples/       MCP Inspector and example client config snippets
tests/          integration-level tests (as they're added)
Dockerfile      multi-stage build → distroless, non-root runtime image
```

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
