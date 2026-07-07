# MCP Inspector usage

The server speaks a real MCP stdio transport, so
[MCP Inspector](https://github.com/modelcontextprotocol/inspector) can
drive it directly — either its interactive UI or its `--cli` mode for
scripted smoke tests. From the repository root:

```sh
go build -o bin/agentic-velociraptor-mcp ./cmd/agentic-velociraptor-mcp
```

## Config for this walkthrough

`examples/client-configs/config.read-only.example.yaml` runs entirely in
**mock mode** (no real Velociraptor connection) so every example below
works with zero further setup, but it has `audit.enabled: true` pointing
at `/var/log/agentic-velociraptor-mcp/audit.jsonl` by default — copy it
and point `audit.path` at somewhere writable first:

```sh
cp examples/client-configs/config.read-only.example.yaml /tmp/inspector-smoke.yaml
mkdir -p /tmp/inspector-audit
sed -i 's#/var/log/agentic-velociraptor-mcp/audit.jsonl#/tmp/inspector-audit/audit.jsonl#' /tmp/inspector-smoke.yaml
```

(On macOS, use `sed -i '' 's#...#...#'` — the in-place flag takes an
explicit, possibly-empty backup suffix argument.)

## Interactive UI

```sh
npx @modelcontextprotocol/inspector \
  ./bin/agentic-velociraptor-mcp \
  --config /tmp/inspector-smoke.yaml \
  --profiles-dir profiles
```

This opens a local web UI where you can browse and call tools by hand.

## `--cli` mode (scripted / non-interactive)

`--cli` mode prints one JSON result per invocation instead of opening a
UI — useful for smoke-testing a build in CI or a terminal. **Order
matters**: Inspector's own flags (`--method`, `--tool-name`) go before
the `--` separator; the server command and its flags go after it. Put
`--tool-arg key=value` (repeatable) **last**, after the server's own
flags, or Inspector's argument parser can swallow the server command
that follows it.

**List all tools** (expect exactly 28 — see
[docs/tool-reference.md](../../docs/tool-reference.md) for the full
inventory):

```sh
npx @modelcontextprotocol/inspector --cli --method tools/list -- \
  ./bin/agentic-velociraptor-mcp --config /tmp/inspector-smoke.yaml --profiles-dir profiles
```

If you see a different count, something has regressed — see
`internal/mcpserver/server_test.go`'s exact-tool-inventory test, which is
meant to catch exactly that.

**Health check** (`velo_health_check`, no arguments; mock mode returns
`mode: "mock"`, `velociraptor_connected: false`, and never makes a
network call):

```sh
npx @modelcontextprotocol/inspector --cli --method tools/call --tool-name velo_health_check -- \
  ./bin/agentic-velociraptor-mcp --config /tmp/inspector-smoke.yaml --profiles-dir profiles
```

**List DFIR profiles** (`velo_list_dfir_profiles`; expect 46 — see
[docs/dfir-profiles.md](../../docs/dfir-profiles.md)):

```sh
npx @modelcontextprotocol/inspector --cli --method tools/call --tool-name velo_list_dfir_profiles -- \
  ./bin/agentic-velociraptor-mcp --config /tmp/inspector-smoke.yaml --profiles-dir profiles
```

**Safe structured error for an unknown profile** (`isError: true`, not a
crash or protocol error):

```sh
npx @modelcontextprotocol/inspector --cli --method tools/call --tool-name velo_get_dfir_profile -- \
  ./bin/agentic-velociraptor-mcp --config /tmp/inspector-smoke.yaml --profiles-dir profiles \
  --tool-arg name=does_not_exist
```

**Profile validation** (`velo_validate_dfir_profile`; `windows_basic_triage`
is valid against the default allowlist):

```sh
npx @modelcontextprotocol/inspector --cli --method tools/call --tool-name velo_validate_dfir_profile -- \
  ./bin/agentic-velociraptor-mcp --config /tmp/inspector-smoke.yaml --profiles-dir profiles \
  --tool-arg name=windows_basic_triage
```

**Write-capable tool refused in default (read-only) mode** — the example
config's `policy.mode` is `read_only`, so every approval-gated tool
reports itself disabled rather than attempting anything:

```sh
npx @modelcontextprotocol/inspector --cli --method tools/call --tool-name velo_collect_artifact_with_approval -- \
  ./bin/agentic-velociraptor-mcp --config /tmp/inspector-smoke.yaml --profiles-dir profiles \
  --tool-arg case_id=CASE-1 --tool-arg reason=smoketest --tool-arg requester=tester \
  --tool-arg approval_reference=none --tool-arg client_id=C.1234abcd5678ef90 \
  --tool-arg artifact=Generic.Client.Info
```

Expect `isError: true` with a message that `policy.mode` must be
`"controlled"`. To actually exercise the approval-gated tools, set
`policy.mode: controlled` and `approval.store_path` in your config and
follow the full flow in [docs/approval-flow.md](../../docs/approval-flow.md)
and the README's
[Read-only vs. controlled mode](../../README.md#read-only-vs-controlled-mode)
section — this still requires a real or fake Velociraptor write client to
go beyond the approval-gate check itself.

## Without Inspector: `go test`

The same tool behavior is exercised without any external tooling by
`internal/mcpserver`'s test suite, using the SDK's in-memory transport
(`mcp.NewInMemoryTransports`) rather than a real subprocess:

```sh
go test ./internal/mcpserver/... -v
```
