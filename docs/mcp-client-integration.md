# MCP client integration notes

How to wire this server into an MCP client host for the controlled
pilot, and how to smoke-test the wiring before trusting it. The
README's [Connect an MCP client](../README.md#connect-an-mcp-client)
section covers more hosts (Hermes, OpenCode); this document is the
operator-focused companion with the verification steps.

## What every integration must preserve

- **stdio only.** The client host launches the binary (or `docker run
  --rm -i ...`) as a child process. No port, no URL, no network
  transport for this server.
- **The client gets tools, not authority.** Whatever the client/agent
  is, it can call 28 tools and nothing else. Writes additionally
  require an approval that **only a human can create, out-of-band, via
  the `approve` CLI** — there is no MCP tool that creates, decides, or
  extends an approval, so an agent cannot self-approve, no matter how
  it phrases the request. See [approval-flow.md](approval-flow.md).
- **Config is the blast-radius control.** Point the client at a config
  built from [examples/config/](../examples/config/) —
  `config.readonly.example.yaml` until the pilot stage explicitly
  calls for `config.controlled.example.yaml`
  ([runbooks/controlled-pilot.md](runbooks/controlled-pilot.md)).

## Claude Desktop

Config file locations and both local-binary and Docker JSON shapes are
in the [README](../README.md#claude-desktop). Minimal local-binary
entry (generic paths — substitute your own):

```json
{
  "mcpServers": {
    "velociraptor": {
      "command": "/path/to/agentic-velociraptor-mcp",
      "args": [
        "--config", "/path/to/config.yaml",
        "--profiles-dir", "/path/to/profiles"
      ]
    }
  }
}
```

## Claude Code

```sh
claude mcp add velociraptor -- /path/to/agentic-velociraptor-mcp \
  --config /path/to/config.yaml \
  --profiles-dir /path/to/profiles
```

Then `claude mcp list` to confirm registration and `/mcp` inside a
session to see the tools. The same JSON shape as Claude Desktop works
in a project's `.mcp.json`.

## MCP Inspector smoke checklist

Run this after every deploy, config change, or version bump — it takes
a few minutes and catches wiring drift before an agent does. Full
Inspector usage (interactive UI, flag ordering caveats) is in
[examples/inspector/README.md](../examples/inspector/README.md).

```sh
npx @modelcontextprotocol/inspector --cli --method tools/list -- \
  /path/to/agentic-velociraptor-mcp --config /path/to/config.yaml --profiles-dir /path/to/profiles
```

- [ ] **Exactly 28 tools listed.** Any other count is a regression —
      compare against [tool-reference.md](tool-reference.md).
- [ ] **No raw-VQL or generic-query tool present.** Nothing named like
      `run_vql`, `query`, `execute`; no tool accepts a query string.
      (Pinned by `internal/mcpserver/server_test.go`'s inventory test,
      but verify the deployed artifact, not just the repo.)
- [ ] **`velo_health_check` returns the expected mode** — `"real"` with
      a healthy server response when `read_api_config_path` is set;
      `"mock"` only if you intended mock mode.
- [ ] **A read tool works**: `velo_search_clients` with a small `limit`
      returns real clients (real mode) and respects the limit.
- [ ] **Writes are gated**: call `velo_collect_artifact_with_approval`
      with a made-up `approval_reference` —
      - in `read_only` mode it must refuse as disabled;
      - in `controlled` mode it must return `status: "not_found"` (no
        approval), and nothing must appear in Velociraptor.
- [ ] **The audit log recorded all of the above**, one event per call,
      at the configured `audit.path`.

## Things not to do

- Don't run the server standalone under a process supervisor "to keep
  it warm" — with no client on stdio it just waits; supervision belongs
  to the client host (see
  [production-deployment.md](production-deployment.md)).
- Don't share one config file between a read-only integration and a
  controlled-pilot integration; give each client entry its own config
  so posture is visible per integration.
- Don't put approval store or audit paths anywhere the MCP client's
  user account can write outside the service boundary — the store must
  be writable by the human operator's `approve` CLI and readable by the
  server, and nothing else.
