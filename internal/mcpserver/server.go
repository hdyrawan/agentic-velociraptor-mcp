// Package mcpserver wires the Velociraptor-facing packages
// (config, policy, approval, audit, dfir, velociraptor, vql) into an MCP
// server exposing the stable 24-tool core described in PROJECT_PLAN.md.
//
// As of v0.1.0-alpha.2, only 4 read-only tools are registered:
// velo_health_check, velo_list_dfir_profiles, velo_get_dfir_profile, and
// velo_validate_dfir_profile. velo_health_check can now make a real
// Velociraptor gRPC call (via Deps.ReadClient) when
// Deps.VelociraptorReadMode is "real"; every other tool, and
// velo_health_check itself when in "mock" mode, still make no
// Velociraptor call. The remaining 20 planned tools exist only as
// ToolSpec metadata in tools_flows.go, tools_collection.go,
// tools_hunts.go, and tools_ioc.go and are deliberately NOT registered
// with the MCP server — an unimplemented tool must never be callable.
package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/config"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/dfir"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/policy"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

// Velociraptor read-connectivity modes. See Deps.VelociraptorReadMode.
const (
	VelociraptorModeMock = "mock"
	VelociraptorModeReal = "real"
)

// Deps bundles everything a tool handler needs. Handlers receive this
// (or fields of it) rather than reaching for globals, so tests can
// substitute fakes for every dependency.
type Deps struct {
	Config    *config.Config
	Policy    *policy.Engine
	Audit     audit.Sink
	Approvals approval.Store
	Profiles  *dfir.Registry

	// ReadClient uses config.VelociraptorConfig.ReadAPIConfigPath.
	// velo_health_check calls ReadClient.HealthCheck when
	// VelociraptorReadMode is "real". No other tool calls it yet.
	ReadClient velociraptor.Client

	// VelociraptorReadMode is "real" if ReadClient is backed by an
	// actual mTLS gRPC connection (velociraptor.read_api_config_path was
	// configured and loaded successfully), or "mock" otherwise. Tool
	// handlers must consult this — not attempt to type-assert
	// ReadClient, and not infer it from whether a call happened to
	// succeed — to decide what "mode" to report, since a real client can
	// still fail an individual call (server down, timeout, ...) without
	// that meaning it's in mock mode.
	VelociraptorReadMode string

	// WriteClient uses config.VelociraptorConfig.WriteAPIConfigPath and
	// must only be invoked by handlers after a confirmed approval. Not
	// used by any tool registered as of v0.1.0-alpha.2 — no code path in
	// this milestone reads WriteAPIConfigPath at all.
	WriteClient velociraptor.Client
}

// ToolSpec documents one planned MCP tool. Each tools_*.go file
// declares a []ToolSpec for its tool group, used for documentation
// generation (docs/tool-reference.md). A ToolSpec being present here
// does NOT mean it is registered with the MCP server — compare against
// the register* functions actually called from New to see what is
// callable in a given release. Tool minimization is intentional: see
// docs/security-model.md.
type ToolSpec struct {
	Name             string
	Description      string
	RequiresApproval bool
	ReadOnly         bool
}

// Server wraps the official MCP Go SDK server with this project's
// dependencies and tool registrations.
type Server struct {
	mcp  *mcp.Server
	deps Deps
}

// New constructs a Server and registers the tools that are safe and
// implemented for the current release. Only registerVisibilityTools and
// registerProfileTools are called as of v0.1.0-alpha.1; registering the
// remaining tool groups is deferred to their respective milestones
// (v0.2.0, v0.3.0, v0.4.0) in PROJECT_PLAN.md.
func New(name, version string, deps Deps) *Server {
	s := mcp.NewServer(&mcp.Implementation{Name: name, Version: version}, nil)

	registerVisibilityTools(s, deps)
	registerProfileTools(s, deps)

	return &Server{mcp: s, deps: deps}
}

// Run serves the MCP stdio transport until ctx is cancelled or the
// client disconnects. Stdio is the only transport this project
// supports; see docs/security-model.md's MCP security section for why
// HTTP/SSE transports are deferred.
func (s *Server) Run(ctx context.Context) error {
	if err := s.mcp.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcpserver: run: %w", err)
	}
	return nil
}

// recordAudit sets Timestamp and writes evt to deps.Audit. Audit write
// failures are not surfaced to the MCP caller (a broken audit sink must
// not make the underlying operation appear to fail differently than it
// did), but are also not silently possible to lose track of forever;
// see the TODO below.
//
// TODO(v0.5.0): decide on a fallback (e.g. stderr line) if deps.Audit.Write
// itself errors, so a misconfigured audit path is discoverable at
// startup/operation time rather than only via missing log entries.
func recordAudit(deps Deps, evt audit.Event) {
	if deps.Audit == nil {
		return
	}
	evt.Timestamp = time.Now().UTC()
	_ = deps.Audit.Write(evt)
}

// boolPtr is a small helper for the *bool fields of mcp.ToolAnnotations.
func boolPtr(b bool) *bool {
	return &b
}

// readOnlyAnnotations returns the ToolAnnotations shared by every tool
// registered in this milestone: all four are read-only, non-destructive,
// and closed-world (they only ever look at local config/registry state
// or a static mock, never an open-ended external system).
func readOnlyAnnotations(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    true,
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(false),
		IdempotentHint:  true,
	}
}
