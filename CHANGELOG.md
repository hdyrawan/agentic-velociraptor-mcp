# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
versioning will follow [SemVer](https://semver.org/) once tagged
releases begin.

## [Unreleased]

## [0.1.0] - 2026-07-06

### Fixed — live lab validation of v0.1.0 read-only visibility (2026-07-06)

- Validated the 8-tool read-only surface against a real disposable
  Velociraptor lab (version 0.76.3) using its generated least-privilege
  `reader` API identity, over a real stdio subprocess — not just unit
  tests against fakes. See docs/lab-validation-plan.md's Phase 1/Phase 2
  sections and PROJECT_STATE.md's "Live lab validation" section for full
  results, including what remains unverified (the lab had zero enrolled
  endpoint clients).
- **Fixed a real bug found by this validation**:
  `internal/velociraptor/grpcclient.go`'s `GetClientInfo` previously
  treated Velociraptor's real `GetClient` response for an unknown client
  ID (a zero-value `ApiClient`, not an error) as a successful lookup,
  surfacing `velo_get_client_info` results with a hollow client object
  (`client_id: ""`) instead of an honest not-found message — a violation
  of this project's evidence-honesty principle. Added
  `ErrClientNotFound` (`internal/velociraptor/clients.go`) and a check
  for `resp.GetClientId() == ""` in `GetClientInfo`; the tool now
  reports `mode: "real"`, no `client` field, and an explanatory message
  instead. Covered by `TestGRPCClientGetClientInfoNotFound`.

### Added — v0.1.0 read-only Velociraptor visibility

- Implemented the four remaining read-only visibility tools:
  `velo_search_clients`, `velo_get_client_info`,
  `velo_list_artifact_names`, `velo_get_artifact_details`. The callable
  tool inventory is now exactly 8 (up from 4): all five visibility
  tools plus the three existing DFIR profile tools.
- `internal/velociraptor/veloapi/`: added `visibility.proto` (message
  definitions for `SearchClientsRequest`/`Response`, `ApiClient`,
  `AgentInformation`, `Uname`, `GetClientRequest`,
  `GetArtifactsRequest`, `ArtifactDescriptors`, `Artifact`,
  `ArtifactParameter`) and `api.proto` (the `service API` definition,
  moved out of `health.proto`, now declaring `Check`, `ListClients`,
  `GetClient`, and `GetArtifacts`). Field names/numbers copied from
  upstream Velociraptor's `api/proto/clients.proto`,
  `api/proto/artifacts.proto`, and `artifacts/proto/artifact.proto`
  (fetched directly from `Velocidex/velociraptor` on GitHub on
  2026-07-05). `Artifact` and its `ArtifactParameter` deliberately omit
  every field carrying VQL text (`sources`, `precondition`, etc.) — see
  docs/security-model.md's updated "Dependency surface" section.
  Regenerated with `buf generate` (same `protoc-gen-go`/`protoc-gen-go-grpc`
  toolchain as `health.proto`).
- Chose Velociraptor's purpose-built `ListClients`, `GetClient`, and
  `GetArtifacts` gRPC RPCs over the generic `Query` (streaming VQL) RPC
  for all four new tools: every caller-supplied value (search query,
  client ID, artifact name) is bound as a plain protobuf field, never a
  VQL string or parameter, so none of `internal/vql`'s
  template/parameter-binding machinery was needed for this milestone.
- `internal/velociraptor/grpcclient.go`: `grpcClient` gained
  `SearchClients`, `GetClientInfo`, `ListArtifactNames`, and
  `GetArtifactDetails`, each timeout-bounded via the existing
  `c.timeout` and routed through the existing `sanitizeTLSError`.
  `NewGRPCClient` gained a `maxRows` parameter
  (`config.VelociraptorConfig.MaxRows`) used to bound
  `SearchClients`/`ListArtifactNames` result counts server- and
  client-side (`boundLimit`/`effectiveMaxRows`; a non-positive value
  falls back to an internal `defaultMaxRows = 100` rather than being
  unbounded). Three new narrow seam interfaces (`clientSearcher`,
  `clientGetter`, `artifactCatalog`), mirroring the existing
  `healthChecker` pattern, keep every new method testable against fakes
  with no real TLS/network setup.
- `internal/validation`: added `SearchQuery` (length cap, rejects
  control characters) for `velo_search_clients`'s free-text filter —
  defense in depth only, since the query is always bound as a plain
  protobuf field, never concatenated into anything.
- `internal/policy`: added `Engine.AllowListAllArtifacts()`, backing
  `velo_list_artifact_names`'/`velo_get_artifact_details`' allowlist
  gating (`policy.allow_list_all_artifacts`); `ArtifactAllowed` remains
  the sole gate for actually *using* an artifact in a future
  collection/hunt.
- `internal/mcpserver/tools_visibility.go`: all four new handlers follow
  `velo_health_check`'s existing evidence-honesty pattern — every
  response carries a `mode` field (`"mock"`/`"real"`), connectivity or
  lookup failures are reported as a normal structured result (empty/`nil`
  data plus a `message`) rather than a Go-level tool error, and only
  input-validation failures (malformed client ID/artifact name/search
  query) or policy-allowlist blocks return a Go-level error with a
  `blocked` audit outcome. Every call still produces exactly one audit
  event.
- `cmd/agentic-velociraptor-mcp`: `buildDeps` now passes
  `cfg.Velociraptor.MaxRows` into `NewGRPCClient`.
- Tests: `grpcclient_test.go` gained fakes (`fakeClientSearcher`,
  `fakeClientGetter`, `fakeArtifactCatalog`) and success/error/timeout/
  limit-bounding/no-secret-leakage tests for all four new methods.
  `tools_visibility_test.go` gained mock-mode, real-mode
  success/error, invalid-input, and allowlist-gating tests for all four
  new handlers, plus a `fakeVisibilityClient`. `server_test.go`'s
  exact-inventory test now expects 8 tools, and gained
  `TestNewNeverRegistersUnsafeTools` (no collect/hunt/download/cancel/
  vql-named tool is ever callable) and an MCP-session-level call test
  for all four new tools. `internal/validation` gained `TestSearchQuery`.
  De-brittled two pre-existing, out-of-scope test assertions
  (`dfir.TestLoadDirParsesShippedProfiles`,
  `mcpserver.TestListDFIRProfilesHandlerReturnsShippedProfiles`) that
  hardcoded an exact profile count against `profiles/`'s contents, which
  had already grown independently of this milestone; both now assert
  "at least" the known profiles load, per this milestone's constraint
  not to touch `profiles/` or `docs/dfir-profiles.md`.
- Docs: README, PROJECT_STATE, docs/tool-reference.md,
  docs/security-model.md (extended "Dependency surface" and "Evidence
  honesty" sections), docs/lab-validation-plan.md (Phase 2 filled in),
  docs/configuration.md (`read_api_config_path`/`max_rows` field notes
  updated for the new tools).

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
