# Project state

Last updated: 2026-07-05 (v0.1.0-alpha.2).

## Current milestone

**v0.1.0-alpha.2 — Real Velociraptor health check.** Complete. Next:
v0.1.0 (the remaining read-only visibility tools: search clients, get
client info, list/get artifacts, flows).

**Requires Go 1.25+ to build.**

## What exists

- Go module `github.com/hdyrawan/agentic-velociraptor-mcp` (go 1.25).
- Dependencies: `gopkg.in/yaml.v3`, `github.com/modelcontextprotocol/go-sdk`
  v1.6.1 (MCP protocol), `google.golang.org/grpc` v1.82.0 and
  `google.golang.org/protobuf` v1.36.11 (new this milestone, for the
  real Velociraptor gRPC client), plus their transitive deps. No other
  direct dependencies.
- `cmd/agentic-velociraptor-mcp`: CLI with `--version`, `--help`,
  `--config` (required to run), `--profiles-dir` (default `profiles`,
  now falls back to a path relative to the executable if the
  cwd-relative lookup fails — see `resolveProfilesDir`). With a valid
  `--config`, the default command loads config, builds all
  dependencies (constructing a real Velociraptor gRPC client if
  `velociraptor.read_api_config_path` is set, or the mock placeholder
  if it's empty), and runs a real MCP server over stdio. A missing
  config file, or a *configured-but-broken* `read_api_config_path`,
  both fail closed (exit 1) without ever starting the transport.
- `internal/mcpserver`: same 4 registered tools as v0.1.0-alpha.1
  (`velo_health_check`, `velo_list_dfir_profiles`,
  `velo_get_dfir_profile`, `velo_validate_dfir_profile`) — no tool was
  added or removed. `velo_health_check`'s handler now branches on
  `Deps.VelociraptorReadMode`:
  - `"mock"` (default, `read_api_config_path` empty): same static
    response as v0.1.0-alpha.1.
  - `"real"`: calls `Deps.ReadClient.HealthCheck(ctx)`. Success reports
    `mode: "real"`, `velociraptor_connected: true`. Failure (timeout,
    transport error, `NOT_SERVING`) reports `status: "error"`,
    `velociraptor_connected: false`, and a safe message — as a normal
    successful tool result, not a Go-level error, per
    docs/security-model.md's evidence-honesty principle.
  The other 20 planned tools remain unregistered `ToolSpec` metadata,
  same as before; the exact-tool-inventory test still passes.
- `internal/velociraptor`: gained real capability this milestone.
  - `apiconfig.go`: `LoadAPIConfig` reads a real Velociraptor
    `api.config.yaml`, fails closed on a missing/non-regular/overly
    permissive (POSIX) file or missing required fields.
    `APIConfig.String()`/`GoString()` are hard-redacted.
  - `veloapi/`: a small, hand-authored, buf-generated gRPC client for
    exactly one RPC — `API.Check` — wire-compatible with a real
    Velociraptor server but sharing no code or dependency with the
    upstream `Velocidex/velociraptor` module. See `health.proto`'s doc
    comment for the regeneration command
    (`buf generate internal/velociraptor/veloapi`).
  - `grpcclient.go`: `NewGRPCClient` builds the mTLS connection;
    `grpcClient` (embeds the existing placeholder for every
    not-yet-implemented method) implements a real `HealthCheck`,
    timeout-bounded, via the `Check` RPC. `sanitizeTLSError` strips any
    PEM-shaped content from gRPC/TLS error text as defense in depth.
  - `client.go`'s placeholder/`NewClient()` is unchanged and still used
    for `WriteClient` unconditionally, and for `ReadClient` when mock
    mode is selected.
- `internal/config`: `read_api_config_path` is now optional
  (`config.Validate` no longer requires it) — empty is a valid,
  supported "mock mode" configuration. `write_api_config_path` was
  already optional and remains untouched by any code path.
- Tests: `internal/velociraptor` gained `apiconfig_test.go` and
  `grpcclient_test.go` (fakes only — no real TLS/network in any test).
  `internal/mcpserver` gained real-mode success/error tests for
  `velo_health_check`, using a `fakeVelociraptorClient` that embeds
  `velociraptor.NewClient()` and overrides just `HealthCheck` (same
  embedding pattern as the production `grpcClient`). `cmd/...` gained
  tests for a broken configured read API path (fails closed),
  mock-mode `buildDeps`, and all three `resolveProfilesDir` branches.
  All tests use fakes/fixtures — none require a live Velociraptor
  server or real certificates.
- Manually verified once (not part of `go test`; see
  docs/lab-validation-plan.md Phase 1): built binary, mock-mode config,
  real subprocess over stdio via the SDK's `CommandTransport` — exactly
  4 tools listed, `velo_health_check` reports `mode: "mock"` honestly.
  **Not yet verified against a real Velociraptor server** — no lab
  server was available in this environment; see the "Known assumptions"
  and lab-validation-plan.md's unchecked Phase 1 items below.
- Docs updated for this milestone: README, PROJECT_PLAN,
  docs/configuration.md (new "read API config file" section),
  docs/security-model.md (secrets handling detail, new "Dependency
  surface" section explaining why `Velocidex/velociraptor` isn't
  imported), docs/lab-validation-plan.md (Phase 1 split), 
  docs/velociraptor-permissions.md (Check RPC needs no ACL beyond a
  valid cert), docs/tool-reference.md, examples/client-configs (new
  `reader.api.config.example.yaml`, mock-mode-by-default main example),
  examples/inspector/README.md.

## What does not exist yet

- Any of the other 20 stable-core tools as callable MCP tools
  (visibility beyond health check, flows, collection, hunts, IOC).
- Any RPC beyond `Check` — no `Query`, no artifact listing, no client
  search, all still `ErrNotImplemented` on `grpcClient` (delegated to
  the embedded placeholder).
- Any approval mechanism implementation (types only; `Deps.Approvals`
  still `nil`).
- Any audit sanitizer implementation beyond a flat top-level map
  redactor.
- Any write-capable Velociraptor client — `WriteClient` is always the
  mock placeholder; `write_api_config_path` is not read by any code.
- HTTP/SSE/streamable HTTP transport — intentionally out of scope.
- 12 of the 15 planned DFIR profiles.
- Real-server validation of the health check (see above) — the next
  person with access to a disposable Velociraptor lab server should run
  through docs/lab-validation-plan.md's unchecked Phase 1 items.
- Docker image, rate limiting, further integration tests (v0.5.0
  concerns).

## Known assumptions to revisit

- Go module path assumed as `github.com/hdyrawan/agentic-velociraptor-mcp`
  (not confirmed against an actual GitHub remote at time of writing).
- `internal/validation.ClientID`'s regex (`C.` + 16 hex chars) is not yet
  confirmed against a live Velociraptor server's actual client ID format.
- Artifact names used in `profiles/windows_ransomware_triage.yaml` and
  `profiles/linux_basic_triage.yaml` are illustrative and unverified.
- No Velociraptor-side ACL name list has been confirmed against a
  specific Velociraptor server version.
- **The real gRPC health check has not been exercised against a real
  Velociraptor server.** Its correctness rests on: (a) the upstream
  `.proto` field names/numbers I fetched directly from
  `Velocidex/velociraptor`'s GitHub repo via `gh api`/`gh search code`
  (health.proto, api.proto, config.proto, grpc_client/grpc.go,
  api/health.go, utils/users.go — all read on 2026-07-05), and (b) unit
  tests against fakes. If a real server behaves differently (e.g. a
  Velociraptor version with a changed `Check` handler, or additional
  auth requirements this reading missed), that would only surface
  against a live server. Recommend lab validation (Phase 1's unchecked
  items) before relying on this in any real deployment.
- `--profiles-dir`'s executable-relative fallback
  (`resolveProfilesDir`) is new and only tested via temp-directory
  fixtures in `cmd/agentic-velociraptor-mcp/main_test.go`, not against
  a packaged/installed binary layout.
- go.mod's `go` directive is 1.25.0, auto-selected when
  `modelcontextprotocol/go-sdk` was added in v0.1.0-alpha.1; confirm
  this is acceptable for your build/deploy environment.

## Immediate next step

v0.1.0: implement the remaining read-only visibility and flow tools
(`velo_search_clients`, `velo_get_client_info`,
`velo_list_artifact_names`, `velo_get_artifact_details`,
`velo_list_flows`, `velo_get_flow_status`, `velo_get_flow_results`).
Each will need its own minimal `veloapi` RPC addition (following the
same pattern as `Check`: fetch the exact upstream message/service shape
first, add only the fields actually used, generate with `buf`) since
none of those RPCs are implemented in `veloapi` yet. Consider using this
milestone as the point to finally validate the real health check
against a disposable lab Velociraptor server, before building more
capability on top of an unvalidated foundation.
