# agentic-velociraptor-mcp

A secure-by-design [MCP](https://modelcontextprotocol.io) server that exposes
safe, auditable, policy-controlled [Velociraptor](https://docs.velociraptor.app/)
endpoint DFIR capabilities to MCP-compatible agents — endpoint visibility,
collection flows/results, reviewed DFIR investigation profiles, a
controlled, approval-gated single-client collection pilot, hunt
management, and a fixed-template IOC hunting helper.

**Status: v1.0.0 — first production release, for strict controlled
deployment.** The safe production posture is built in and non-negotiable:
**read-only by default**; `controlled` mode activates write-capable
tools only with a human out-of-band **approval** for every write,
exact-name artifact/profile **allowlists**, a fail-closed JSONL **audit
log**, and separate **least-privilege Velociraptor API identities**
(never `administrator`). Deploy through
[docs/release/v1.0.0-production-checklist.md](docs/release/v1.0.0-production-checklist.md).

The release ships 28 callable MCP tools: 14 read-only (visibility,
DFIR profiles, workflow helpers, flows/results), plus 6 approval-gated
write tools implementing a controlled single-client collection pilot
(collect artifact/profile, cancel flow, list/get/download flow uploads),
plus 7 hunt management tools (preview, start, start-DFIR, list, status,
results, cancel), plus 1 IOC hunting helper
(`velo_hunt_ioc_with_approval`, for hash/ip/domain/process/path
indicators). Hunt tools include read-only scope preview, list, status,
and results (mock/real branching) plus approval-gated start, start-DFIR,
cancel, and IOC hunt — all gated through approval, policy, scope
validation, artifact/profile/template allowlists, and
`max_hunt_clients` enforcement. A curated catalog of 46 DFIR profiles
ships under `profiles/`, every artifact backed by a reviewed entry in
`catalog/artifacts.yaml` (see [docs/dfir-profiles.md](docs/dfir-profiles.md)).

**Backend status:** every one of the 28 tools has a real typed
Velociraptor gRPC RPC binding (health, client search/info, artifact
list/details, flow list/status/results, collection start/cancel, flow
uploads/download, and hunt list/status/results/preview/start/cancel) —
see `internal/velociraptor/grpcclient*.go`. No raw/generic VQL query
path exists or is planned for the stable core. **Known limitation:** an
explicit `client_ids` hunt scope has no typed Velociraptor RPC support in
real mode (only label or all-clients scoping is possible; see
`velociraptor.ErrHuntScopeClientIDsUnsupported`) — the three hunt/IOC
hunt-start tools detect this and leave the approval unconsumed rather
than burning it on a call that can't succeed. **Live-lab validation of
the write-capable paths (collection, hunts) against a real Velociraptor
server was performed in v0.10.2** — see
[docs/live-validation-report-v0.10.2.md](docs/live-validation-report-v0.10.2.md)
for the full pass/fail detail. It found two real correctness bugs,
**both fixed in v0.10.3**: `velo_get_flow_results`/`velo_get_hunt_results`
now correctly handle named-source artifacts (notably
`Generic.Client.Info`) via an optional `source` input and a
`status: "source_required"` response when disambiguation is needed
instead of silently reporting empty; `velo_hunt_ioc_with_approval`'s
`kind: "hash"` now resolves to a real, catalog-verified artifact
(`Generic.Detection.HashHunter`, confirmed via a real `CreateHunt` call),
while `ip`/`domain`/`process`/`path` fail closed with a clear
unsupported-kind error before any approval is consumed rather than an
invented artifact name — see
[docs/tool-reference.md](docs/tool-reference.md) for the full behavior.
Uploads/downloads and Windows-client/label-scoped-hunt paths remain
unvalidated live — see
[docs/lab-validation-plan.md](docs/lab-validation-plan.md).

**The operational material for production deployment** ships in-repo:
production-safe config examples ([examples/config/](examples/config/)),
operational runbooks ([docs/runbooks/](docs/runbooks/) — approvals &
audit, rollback/containment, and the staged first-deployment plan),
deployment hardening
([docs/production-deployment.md](docs/production-deployment.md)),
MCP client integration notes with an Inspector smoke checklist
([docs/mcp-client-integration.md](docs/mcp-client-integration.md)), the
security gate
([docs/security-review-checklist-v0.10.4.md](docs/security-review-checklist-v0.10.4.md)),
and the release-level
[v1.0.0 production checklist](docs/release/v1.0.0-production-checklist.md).
Deploy only through that checklist — start in the read-only posture and
move to `controlled` deliberately. The known limitations accepted for
v1.0.0 are listed in [PROJECT_STATE.md](PROJECT_STATE.md) and in the
checklist itself — see [PROJECT_PLAN.md](PROJECT_PLAN.md) for the
roadmap.

## Contents

- [Why this exists](#why-this-exists)
- [Quick start](#quick-start)
  - [Option A: Docker](#option-a-docker)
    - [1. Build the image](#1-build-the-image)
    - [2a. Smoke-test the container in mock mode](#2a-smoke-test-the-container-in-mock-mode)
    - [2b. Create a real read-only config](#2b-create-a-real-read-only-config)
    - [3. Run the hardened Docker command](#3-run-the-hardened-docker-command)
    - [4. Register with Hermes](#4-register-with-hermes)
  - [Option B: Build from source](#option-b-build-from-source)
- [Configure the Velociraptor connection](#configure-the-velociraptor-connection)
- [Read-only vs. controlled mode](#read-only-vs-controlled-mode)
- [Connect an MCP client](#connect-an-mcp-client)
  - [Claude Desktop](#claude-desktop)
  - [Claude Code](#claude-code)
  - [Hermes](#hermes)
  - [OpenCode](#opencode)
  - [Any other MCP client / MCP Inspector](#any-other-mcp-client--mcp-inspector)
- [Common workflows](#common-workflows)
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

You need an MCP client to launch this server. The server itself speaks
MCP over stdio only: it does **not** listen on a port, and a standalone
`docker run` waits on stdin until an MCP client talks to it.

The fastest path is Docker in read-only mode:

1. build the image;
2. create a host config directory;
3. mount a Velociraptor `api.config.yaml` secret;
4. register the `docker run ...` command with your MCP client.

If you only want to confirm the container starts, use the mock-mode
example in [Step 2a](#2a-smoke-test-the-container-in-mock-mode). If you
want real Velociraptor visibility, continue through
[Step 2b](#2b-create-a-real-read-only-config).

### Option A: Docker

#### 1. Build the image

```sh
git clone https://github.com/hdyrawan/agentic-velociraptor-mcp.git
cd agentic-velociraptor-mcp
docker build -t agentic-velociraptor-mcp:v0.10.4 .
```

For local experiments you can also tag `:latest`, but production and
pilot deployments should use a version tag or image digest.

#### 2a. Smoke-test the container in mock mode

This verifies Docker and the binary before you wire in a real
Velociraptor API identity. It does not contact Velociraptor.
The distroless image runs as non-root UID 65532, so the mounted audit
directory must be writable by that UID.

```sh
mkdir -p ./audit
sudo chown -R 65532:65532 ./audit
docker run --rm -i \
  --read-only \
  --cap-drop=ALL \
  --security-opt no-new-privileges \
  -v "$(pwd)/examples/client-configs/config.read-only.example.yaml:/etc/agentic-velociraptor-mcp/config.yaml:ro" \
  -v "$(pwd)/audit:/var/log/agentic-velociraptor-mcp" \
  agentic-velociraptor-mcp:v0.10.4
```

The process will appear to hang because it is waiting for MCP messages
on stdin. Press `Ctrl-C` after this smoke test, or use
[MCP Inspector](examples/inspector/README.md) to call `tools/list` and
`velo_health_check`.

#### 2b. Create a real read-only config

Create host directories for the MCP config, Velociraptor API secret, and
audit log:

```sh
sudo mkdir -p /etc/agentic-velociraptor-mcp/secrets
sudo mkdir -p /var/log/agentic-velociraptor-mcp
sudo chown -R 65532:65532 /etc/agentic-velociraptor-mcp /var/log/agentic-velociraptor-mcp
sudo chmod 700 /etc/agentic-velociraptor-mcp/secrets /var/log/agentic-velociraptor-mcp
```

Create the Velociraptor API client on the Velociraptor server host. This
step produces the `api.config.yaml` file that the MCP container mounts
as its secret. The MCP config never contains the certificate/key content
itself — it only points at this file.

For a read-only integration, start with a dedicated reader identity:

```sh
velociraptor --config /path/to/server.config.yaml config api_client \
  --name agentic-velociraptor-mcp-reader \
  --role api \
  /tmp/reader.api.config.yaml
```

Then grant only the read permissions/role your Velociraptor version uses
for client search, client metadata, artifact catalog browsing, and
existing flow/hunt result reads. Depending on your Velociraptor version
and role model, this may be done in the GUI under server user/API-client
management, or with the ACL CLI. Example shape:

```sh
velociraptor --config /path/to/server.config.yaml acl grant \
  agentic-velociraptor-mcp-reader \
  --role api,reader
```

`acl grant` replaces the principal's existing roles unless you pass
`--merge`, and ACL changes made this way may require a Velociraptor
server restart. The Velociraptor GUI or `user_grant` VQL function can
apply user changes at runtime on deployments that support it.

Do **not** use `administrator`, and do not grant `ARTIFACT_WRITER`,
`SERVER_ARTIFACT_WRITER`, `EXECVE`, `FILESYSTEM_WRITE`, or
`SERVER_ADMIN`. See
[docs/velociraptor-permissions.md](docs/velociraptor-permissions.md) for
the least-privilege guidance and version-specific caveats.

Copy the generated API config to the MCP host/container secret path:

```sh
sudo cp /tmp/reader.api.config.yaml \
  /etc/agentic-velociraptor-mcp/secrets/reader.api.config.yaml
```

Lock it down. The server enforces strict permissions on POSIX systems:

```sh
sudo chown 65532:65532 /etc/agentic-velociraptor-mcp/secrets/reader.api.config.yaml
sudo chmod 600 /etc/agentic-velociraptor-mcp/secrets/reader.api.config.yaml
```

Create `/etc/agentic-velociraptor-mcp/config.yaml`:

```yaml
server:
  name: agentic-velociraptor-mcp
  transport: stdio

velociraptor:
  org_id: root
  read_api_config_path: /etc/agentic-velociraptor-mcp/secrets/reader.api.config.yaml
  write_api_config_path: ""
  timeout_seconds: 30
  max_rows: 500
  max_result_bytes: 1048576
  max_upload_bytes: 52428800
  download_dir: ""

policy:
  mode: read_only
  allow_raw_vql: false
  allow_list_all_artifacts: false
  allow_target_all: false
  max_hunt_clients: 25
  require_approval_for:
    - collect_artifact
    - collect_dfir_profile
    - start_hunt
    - start_dfir_hunt
    - cancel_flow
    - cancel_hunt
    - download_flow_upload
    - hunt_ioc
  allowed_artifacts:
    - Generic.Client.Info
    - Generic.Detection.HashHunter
    - Windows.System.Pslist
    - Windows.Network.Netstat
    - Linux.Sys.Pslist
    - Linux.Network.Netstat
  allowed_profiles:
    - windows_basic_triage
    - linux_basic_triage
    - cross_platform_identity
    - cross_platform_ioc_context
    - cross_platform_local_hashes

audit:
  enabled: true
  path: /var/log/agentic-velociraptor-mcp/audit.jsonl
  max_size_bytes: 104857600
  max_files: 10
  redact_fields:
    - client_private_key
    - client_cert
    - ca_certificate
    - api_key
    - approval_token
    - password
    - secret

approval:
  store_path: ""
  ttl_seconds: 900
```

Then set safe ownership:

```sh
sudo chown 65532:65532 /etc/agentic-velociraptor-mcp/config.yaml
sudo chmod 644 /etc/agentic-velociraptor-mcp/config.yaml
```

This read-only posture can list clients, inspect metadata, list existing
flows/hunts, and plan triage. It cannot collect artifacts, start hunts,
cancel flows/hunts, or download evidence.

#### 3. Run the hardened Docker command

This is the command your MCP client should launch:

```sh
docker run --rm -i \
  --read-only \
  --cap-drop=ALL \
  --security-opt no-new-privileges \
  -v "/etc/agentic-velociraptor-mcp/config.yaml:/etc/agentic-velociraptor-mcp/config.yaml:ro" \
  -v "/etc/agentic-velociraptor-mcp/secrets:/etc/agentic-velociraptor-mcp/secrets:ro" \
  -v "/var/log/agentic-velociraptor-mcp:/var/log/agentic-velociraptor-mcp" \
  agentic-velociraptor-mcp:v0.10.4
```

Again, running this directly waits on stdin. Register it with an MCP
client, then call `velo_health_check`; a real deployment should report
`mode: "real"` and a healthy Velociraptor connection.

#### 4. Register with Hermes

Hermes can register the Docker command directly:

```sh
hermes mcp add velociraptor \
  --command docker \
  --args run --rm -i \
    --read-only \
    --cap-drop=ALL \
    --security-opt no-new-privileges \
    -v /etc/agentic-velociraptor-mcp/config.yaml:/etc/agentic-velociraptor-mcp/config.yaml:ro \
    -v /etc/agentic-velociraptor-mcp/secrets:/etc/agentic-velociraptor-mcp/secrets:ro \
    -v /var/log/agentic-velociraptor-mcp:/var/log/agentic-velociraptor-mcp \
    agentic-velociraptor-mcp:v0.10.4
```

Check the registration:

```sh
hermes mcp list
hermes mcp test velociraptor
```

If Hermes is already running, reload or restart the session so the MCP
tool list is refreshed.

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

Either way, the server speaks MCP over stdio and exposes exactly 28
tools (14 read-only, 14 approval-gated or gated) — see
[docs/tool-reference.md](docs/tool-reference.md) for the current
callable inventory.

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

## Read-only vs. controlled mode

`policy.mode` in `config.yaml` picks one of two postures. There is no
partial state in between: every write-capable tool checks this at call
time, not just at startup.

- **`read_only` (the default, and the recommended starting point):**
  every approval-gated tool (collection, hunt start/cancel, IOC hunt,
  flow cancel, evidence download) refuses to run and reports itself
  disabled. Only the 14 read-only tools do anything. Safe to point at a
  real Velociraptor deployment for visibility/triage with zero risk of
  an agent changing endpoint state.
- **`controlled`:** approval-gated tools become reachable, but only once
  **both** `policy.mode: controlled` **and** `approval.store_path` are
  set — either one missing keeps every write tool refusing to run. Even
  then, no MCP tool call executes a write by itself:

  1. An agent (or you, through an MCP client) calls an approval-gated
     tool, e.g. `velo_collect_artifact_with_approval`, supplying
     `case_id`, `reason`, `requester`, and an `approval_reference` — a
     reference to an approval that doesn't exist yet.
  2. The tool call is refused: no matching approval, so nothing runs.
  3. **A human**, not the MCP client, runs the separate `approve` CLI
     subcommand shipped in this binary to create and approve that exact
     request:

     ```sh
     ./bin/agentic-velociraptor-mcp approve \
       --store /path/to/approvals.json --reference CASE-42-collect-1 \
       --operation collect_artifact --case-id CASE-42 \
       --reason "triage per incident #42" --requester analyst@example.com \
       --approved-by lead-analyst@example.com \
       --client-id C.1234abcd5678ef90 --artifact Generic.Client.Info
     ```

  4. The agent retries the same tool call with `approval_reference:
     "CASE-42-collect-1"`. It now succeeds — and only for that exact
     client/artifact/parameters; a mismatched call (different client,
     different artifact) is rejected even with a valid, approved
     reference.

  No MCP tool can create or decide an approval — `approve` is a
  human-operator-only command, never reachable over the MCP stdio
  transport. This is what prevents an LLM-driven agent from approving its
  own request. See [docs/approval-flow.md](docs/approval-flow.md) for the
  full workflow, including denial and expiry.

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
        "--read-only",
        "--cap-drop=ALL",
        "--security-opt", "no-new-privileges",
        "-v", "/etc/agentic-velociraptor-mcp/config.yaml:/etc/agentic-velociraptor-mcp/config.yaml:ro",
        "-v", "/etc/agentic-velociraptor-mcp/secrets:/etc/agentic-velociraptor-mcp/secrets:ro",
        "-v", "/var/log/agentic-velociraptor-mcp:/var/log/agentic-velociraptor-mcp",
        "agentic-velociraptor-mcp:v0.10.4"
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

Hermes can register the same stdio server from the CLI. For the Docker
quick start above:

```sh
hermes mcp add velociraptor \
  --command docker \
  --args run --rm -i \
    --read-only \
    --cap-drop=ALL \
    --security-opt no-new-privileges \
    -v /etc/agentic-velociraptor-mcp/config.yaml:/etc/agentic-velociraptor-mcp/config.yaml:ro \
    -v /etc/agentic-velociraptor-mcp/secrets:/etc/agentic-velociraptor-mcp/secrets:ro \
    -v /var/log/agentic-velociraptor-mcp:/var/log/agentic-velociraptor-mcp \
    agentic-velociraptor-mcp:v0.10.4

hermes mcp list
hermes mcp test velociraptor
```

`--args` must be the last Hermes option; everything after it is passed
to Docker. If Hermes is already running, reload MCP tools or restart the
session after changing server definitions.

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
- Human approval required for every collection, hunt start/cancel, IOC
  hunt, flow cancel, and evidence download.
- MCP-specific hardening: no credential passthrough (no tool accepts API
  keys/certificates/tokens as arguments), no arbitrary URL fetching,
  unimplemented tools are never registered, session IDs are never
  treated as authentication. See
  [docs/security-model.md](docs/security-model.md#mcp-specific-security-practices).

## Repository layout

```
cmd/agentic-velociraptor-mcp/   CLI entrypoint + `approve` subcommand
internal/
  audit/        structured audit events, JSONL sink, recursive secret redaction, rotation
  approval/     human-approval request/decision workflow (cross-process file-locked store)
  catalog/      (see repo-root catalog/) curated artifact catalog loader + validation
  config/       YAML config model + validation
  dfir/         DFIR profile model, registry, validation, artifact-catalog cross-check
  mcpserver/    MCP server + tool registration (28 registered)
  policy/       MCP-layer policy engine (allowlists, approval routing)
  validation/   strict input validation (client IDs, artifacts, IOCs, scope)
  velociraptor/ Velociraptor gRPC client (mTLS): real RPCs for health, client search/detail,
                artifact catalog, flow list/status/results, collection start/cancel, flow
                uploads/download, hunt list/status/results/preview/start/cancel
  velociraptor/veloapi/  hand-scoped gRPC stubs generated from reviewed proto definitions
  vql/          allowlisted VQL template binding — no raw VQL execution
catalog/        curated artifact catalog (artifacts.yaml) — authoring/test-time control
profiles/       46 DFIR profile definitions (YAML)
docs/           architecture, security model, config, tool, and ops docs
examples/       MCP Inspector and example client config snippets
tests/          integration-level tests (as they're added)
Dockerfile      multi-stage build → distroless, non-root runtime image
```

## Common workflows

These are the same MCP tool calls an agent makes, shown as their JSON
arguments — use [MCP Inspector](examples/inspector/README.md) or your
MCP client to actually invoke them. All examples assume
`policy.mode: controlled` and `approval.store_path` set for the
approval-gated ones; read-only tools work in either mode.

**1. Health check** (`velo_health_check`, read-only, works in mock mode
with zero setup):

```json
{}
```

**2. List clients** (`velo_search_clients`, read-only):

```json
{"query": "windows", "limit": 20}
```

**3. Collect an approved artifact from one client**
(`velo_collect_artifact_with_approval`). First get an approval from a
human operator (see [Read-only vs. controlled mode](#read-only-vs-controlled-mode)
above), then call:

```json
{
  "case_id": "CASE-42", "reason": "triage per incident #42",
  "requester": "analyst@example.com", "approval_reference": "CASE-42-collect-1",
  "client_id": "C.1234abcd5678ef90", "artifact": "Generic.Client.Info"
}
```

**4. Start an approved DFIR profile hunt** across a label-scoped set of
clients (`velo_start_dfir_hunt_with_approval`) — always
`velo_preview_hunt_scope` first to see blast radius before requesting
approval:

```json
{"label": "windows", "max_clients": 25}
```

then, with a matching approval created via `approve --operation
start_dfir_hunt --profile windows_basic_triage --label windows`:

```json
{
  "case_id": "CASE-42", "reason": "triage per incident #42",
  "requester": "analyst@example.com", "approval_reference": "CASE-42-hunt-1",
  "profile": "windows_basic_triage", "label": "windows"
}
```

**5. IOC hunt with approval** (`velo_hunt_ioc_with_approval`) for a
hash/IP/domain/process/path indicator, same approval flow, scoped by
`label` or `all` — **explicit `client_ids` scope is not supported by
Velociraptor's typed hunt RPCs in real mode**, use `label` or `all`
instead (see the "Known limitation" note above):

```json
{
  "case_id": "CASE-42", "reason": "hunting a known-bad hash",
  "requester": "analyst@example.com", "approval_reference": "CASE-42-ioc-1",
  "kind": "hash", "value": "d41d8cd98f00b204e9800998ecf8427e", "label": "windows"
}
```

**6. Review and download flow results** (`velo_get_flow_results`,
read-only, then `velo_download_flow_upload_with_approval` for any
attached file):

```json
{"client_id": "C.1234abcd5678ef90", "flow_id": "F.1234abcd5678ef90", "limit": 50}
```

See [docs/tool-reference.md](docs/tool-reference.md) for every tool's
full input/output schema, and [docs/dfir-profiles.md](docs/dfir-profiles.md)
for the 46 available DFIR profiles.

## Documentation

- [llms.txt](llms.txt) — LLM-friendly repository context, safety boundaries, and verification map.
- [docs/architecture.md](docs/architecture.md)
- [docs/security-model.md](docs/security-model.md)
- [docs/approval-flow.md](docs/approval-flow.md)
- [docs/configuration.md](docs/configuration.md)
- [docs/tool-reference.md](docs/tool-reference.md)
- [docs/dfir-profiles.md](docs/dfir-profiles.md)
- [docs/velociraptor-permissions.md](docs/velociraptor-permissions.md)
- [docs/lab-validation-plan.md](docs/lab-validation-plan.md)
- [docs/production-deployment.md](docs/production-deployment.md)
- [docs/mcp-client-integration.md](docs/mcp-client-integration.md)
- [docs/security-review-checklist-v0.10.4.md](docs/security-review-checklist-v0.10.4.md)
- Release: [v1.0.0 production checklist](docs/release/v1.0.0-production-checklist.md) ·
  [v1.0.0 release notes](docs/release/v1.0.0-release-notes.md)
- Runbooks: [approvals & audit](docs/runbooks/approval-and-audit.md) ·
  [rollback/containment](docs/runbooks/rollback.md) ·
  [controlled pilot](docs/runbooks/controlled-pilot.md)

## License

Apache-2.0. See [LICENSE](LICENSE).

## Acknowledgements / prior art

`socfortress/velociraptor-mcp-server` was reviewed as a feature-set
reference point during planning. No code from that project is used
here; this project's security model, tool boundaries, and implementation
are independent.
