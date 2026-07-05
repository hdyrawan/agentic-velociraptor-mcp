# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versioning will follow [SemVer](https://semver.org/) once tagged
releases begin.

## [Unreleased]

### Added — v0.1.0-alpha.2 real Velociraptor health check

- Added `google.golang.org/grpc` and `google.golang.org/protobuf` as
  dependencies.
- New `internal/velociraptor/veloapi/` package: a minimal, hand-authored
  `.proto` (`health.proto`) mirroring only the single gRPC method this
  project calls on a real Velociraptor server — `API.Check`, modeled on
  the standard gRPC health-checking protocol — with field
  names/numbers/service names copied from upstream's own
  `api/proto/health.proto` and `api/proto/api.proto` for wire
  compatibility. Compiled to ordinary generated Go code with `buf` +
  `protoc-gen-go`/`protoc-gen-go-grpc`. Deliberately does **not** import
  the upstream `Velocidex/velociraptor` server module; see
  docs/security-model.md's new "Dependency surface" section.
- `internal/velociraptor/apiconfig.go`: `LoadAPIConfig` reads a real
  Velociraptor `api.config.yaml` (`ca_certificate`, `client_cert`,
  `client_private_key`, `api_connection_string`, `name`,
  `pinned_server_name`), fails closed on a missing file, a non-regular
  file, an overly permissive file mode (must be owner-only on POSIX),
  or missing required fields. `APIConfig.String()`/`GoString()` are
  hard-coded to a redacted placeholder so the type can never leak key
  material through an accidental format/log call.
- `internal/velociraptor/grpcclient.go`: `NewGRPCClient` builds an
  mTLS-authenticated gRPC connection (`tls.X509KeyPair` + CA pool +
  `credentials.NewTLS`, server name pinned to `pinned_server_name` or
  the upstream default `"VelociraptorServer"`) and a `grpcClient` that
  implements `HealthCheck` for real via the `Check` RPC, enforcing
  `timeout_seconds` via `context.WithTimeout`. Every other `Client`
  method remains the fail-closed placeholder (embedded, unchanged). A
  `healthChecker` seam interface lets tests exercise timeout/error
  handling with a fake, without any real TLS/network setup.
- `internal/mcpserver`: `Deps` gained `VelociraptorReadMode`
  (`"mock"`/`"real"`). `velo_health_check`'s handler now calls
  `ReadClient.HealthCheck` in real mode, reporting connectivity
  failures as a normal structured result (`status: "error"`,
  `velociraptor_connected: false`, safe message) rather than a
  Go-level tool error — Velociraptor being unreachable is data, not a
  tool failure. `HealthCheckOutput` gained `server_version` (always
  empty in this milestone: the `Check` RPC carries no version field).
- `internal/config`: `read_api_config_path` is now optional — empty
  means mock mode. `config.Validate` no longer requires it; the
  file-loadability check happens once, eagerly, in
  `cmd/agentic-velociraptor-mcp`'s `buildDeps`.
- `cmd/agentic-velociraptor-mcp`: `buildDeps` constructs a real
  `velociraptor.NewGRPCClient` when `read_api_config_path` is set
  (failing server startup outright, exit 1, if that config is
  missing/invalid/unsafe — never silently falling back to mock), or the
  mock placeholder when it's empty. `write_api_config_path` remains
  untouched by every code path. Added a small `resolveProfilesDir`
  helper: the default `--profiles-dir` now also tries a path relative to
  the running executable if the cwd-relative lookup fails, so the
  built binary depends less on being invoked from the repo root; an
  explicit `--profiles-dir` is always honored as given.
- Tests: `internal/velociraptor` gained `apiconfig_test.go` (missing
  file, empty path, directory, overly permissive mode, missing fields,
  valid parse, no secret leakage from `String`/`GoString`) and
  `grpcclient_test.go` (success, `NOT_SERVING`, transport error,
  timeout enforcement, no secret leakage via `sanitizeTLSError`, all
  against a fake `healthChecker`). `internal/mcpserver` gained real-mode
  success/error tests for `velo_health_check` (including a check that a
  connectivity failure produces a normal result, not `IsError`/a Go
  error, and that no PEM content reaches the output or audit log).
  `cmd/agentic-velociraptor-mcp` gained tests for a broken configured
  `read_api_config_path` (fails closed), mock-mode `buildDeps`, and
  `resolveProfilesDir`'s three branches. Manually smoke-tested mock mode
  end to end over a real stdio subprocess (see docs/lab-validation-plan.md
  Phase 1).
- `go.mod`'s `go` directive requires Go 1.25+ (unchanged from
  v0.1.0-alpha.1, now load-bearing for `google.golang.org/grpc`/`protobuf`
  too).
- Docs: README, PROJECT_PLAN, PROJECT_STATE, docs/configuration.md (new
  "The read API config file" section), docs/security-model.md (updated
  secrets-handling and evidence-honesty sections, new "Dependency
  surface" section), docs/lab-validation-plan.md (Phase 1 split into
  done-via-fakes vs. needs-a-real-lab-server), docs/velociraptor-permissions.md
  (Check RPC needs no ACL beyond a valid cert; `pinned_server_name`
  note), examples/client-configs (mock-mode-by-default example config,
  new `reader.api.config.example.yaml`), examples/inspector/README.md.

### Added — v0.1.0-alpha.1 MCP skeleton

- Added `github.com/modelcontextprotocol/go-sdk` v1.6.1 as a dependency
  (Go module `go` directive auto-bumped 1.23.4 → 1.25.0 by the SDK's
  minimum Go version requirement).
- `internal/mcpserver.Server` now wraps a real `*mcp.Server` and serves
  the stdio transport (`mcp.StdioTransport`) via `Server.Run(ctx)`,
  replacing the v0.0.x placeholder that panicked.
- Registered exactly 4 callable MCP tools (of the 24 planned):
  `velo_health_check` (static mock — no Velociraptor call yet),
  `velo_list_dfir_profiles`, `velo_get_dfir_profile`,
  `velo_validate_dfir_profile`. All four are read-only,
  non-destructive, closed-world per their `ToolAnnotations`, and audit
  every call. The remaining 20 tools stay unregistered `ToolSpec`
  metadata — unimplemented tools are never made callable.
- `cmd/agentic-velociraptor-mcp`: the default command (given `--config`)
  now loads and validates config, builds the DFIR profile registry, the
  audit sink, and the policy engine, and runs the MCP server over stdio
  until the client disconnects or SIGINT/SIGTERM. Added `--profiles-dir`
  flag. A missing/invalid config file fails closed (exit 1) without ever
  starting the transport.
- Tests: `internal/mcpserver` gained an in-memory-transport test
  asserting the exact 4-tool inventory and read-only annotations, plus
  unit tests for each handler (health check mock output; profile
  list/get/validate including not-found and invalid-name-syntax safe
  error paths; audit outcome assertions via a fake sink).
  `cmd/agentic-velociraptor-mcp` gained a test for the fail-closed
  missing-config-file path. Manually verified once end-to-end over a
  real stdio subprocess via the SDK's `CommandTransport` (see
  docs/lab-validation-plan.md Phase 0).
- Docs: README, PROJECT_PLAN (MCP Security Best-Practice Integration
  section), docs/tool-reference.md, docs/security-model.md (new
  "MCP-specific security practices" section: no credential passthrough,
  no arbitrary URL fetching, unimplemented tools never registered,
  confused-deputy mitigation via approval fingerprinting, tool/scope
  minimization), docs/lab-validation-plan.md (new Phase 0).

### Added — v0.0.x project foundation

- Repository skeleton: `cmd/`, `internal/{audit,approval,config,dfir,
  mcpserver,policy,validation,velociraptor,vql}`, `profiles/`, `docs/`,
  `examples/`, `tests/`.
- Go module `github.com/hdyrawan/agentic-velociraptor-mcp` (Go 1.23).
- CLI entrypoint with `--version` and `--help`; no MCP server behavior
  yet.
- `internal/config`: full config struct tree, YAML loader, structural
  validator, conservative `Default()`.
- `internal/audit`: audit event model with exhaustive
  success/blocked/error outcomes, JSONL sink, redaction placeholder.
- `internal/approval`: approval request/decision model, store interface,
  fingerprinting helper. No approval mechanism implemented yet.
- `internal/policy`: policy engine over config, allow/require-approval/
  deny decision model, dangerous-permissions checklist.
- `internal/dfir`: DFIR profile model, YAML-backed registry, artifact
  allowlist cross-check validation.
- `internal/validation`: client ID, artifact name, DFIR profile name,
  hash/IP/domain, and hunt scope validators.
- `internal/velociraptor`: `Client` interface (health/clients/artifacts/
  flows/hunts/uploads) with a fail-closed placeholder implementation; no
  real gRPC connection yet.
- `internal/vql`: allowlisted IOC-hunt template constants and a
  fail-closed `Bind`; no VQL string construction or execution.
- `internal/mcpserver`: tool metadata (`ToolSpec`) for all 24 planned
  stable-core tools, grouped by concern; server/deps shape defined,
  `Run` intentionally panics until v0.1.0-alpha.1.
- 3 of 15 planned DFIR profile definitions
  (`windows_basic_triage`, `windows_ransomware_triage`,
  `linux_basic_triage`).
- Documentation: architecture, security model, approval flow,
  configuration, tool reference, DFIR profiles, Velociraptor
  permissions, lab validation plan, production deployment.
- Apache-2.0 LICENSE, README, PROJECT_PLAN, PROJECT_STATE.
