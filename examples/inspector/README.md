# MCP Inspector usage

As of v0.1.0-alpha.1, the server runs a real MCP stdio transport. From
the repository root:

```sh
go build -o bin/agentic-velociraptor-mcp ./cmd/agentic-velociraptor-mcp

npx @modelcontextprotocol/inspector \
  ./bin/agentic-velociraptor-mcp \
  --config examples/client-configs/config.read-only.example.yaml \
  --profiles-dir profiles
```

`config.read-only.example.yaml` leaves `read_api_config_path` empty, so
this works out of the box in mock mode. To exercise a real Velociraptor
health check instead, point `read_api_config_path` at a real
`api.config.yaml` (see `reader.api.config.example.yaml` in the same
directory and docs/configuration.md's "The read API config file"
section) before starting Inspector.

You should see exactly 4 tools in Inspector's tool list:

- `velo_health_check` — call with no arguments.
  - Mock mode (`read_api_config_path` empty): returns
    `status: "ok"`, `mode: "mock"`, `velociraptor_connected: false`. No
    Velociraptor connection is made.
  - Real mode (`read_api_config_path` set to a real, valid
    `api.config.yaml`): calls Velociraptor's `Check` gRPC RPC and
    returns `mode: "real"` with `velociraptor_connected: true` on
    success, or `status: "error"`/`velociraptor_connected: false` with a
    safe explanatory `message` if the server is unreachable or the
    check times out — either way as a normal successful tool result,
    not a crash or protocol error.
- `velo_list_dfir_profiles` — call with no arguments; returns the 3
  DFIR profiles shipped under `profiles/`.
- `velo_get_dfir_profile` — call with `{"name": "windows_basic_triage"}`;
  try an unknown name (e.g. `{"name": "does_not_exist"}`) to see the
  safe structured error (`isError: true`, not a crash or protocol
  error).
- `velo_validate_dfir_profile` — call with `{"name": "windows_basic_triage"}`
  (valid against the default artifact allowlist) or
  `{"name": "windows_ransomware_triage"}` (invalid — that profile
  references artifacts not in the default allowlist, so `valid: false`
  with an explanatory `error` field is the expected, correct response,
  not a tool failure).

No other tool should appear. If you see more than these 4, something has
regressed — see `internal/mcpserver/server_test.go`'s exact-tool-inventory
test, which is meant to catch exactly that.

## Without Inspector: `go test`

The same behavior is exercised without any external tooling by
`internal/mcpserver`'s test suite, using the SDK's in-memory transport
(`mcp.NewInMemoryTransports`) rather than a real subprocess:

```sh
go test ./internal/mcpserver/... -v
```
