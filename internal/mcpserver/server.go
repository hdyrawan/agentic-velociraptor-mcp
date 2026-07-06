// Package mcpserver wires the Velociraptor-facing packages
// (config, policy, approval, audit, dfir, velociraptor, vql) into an MCP
// server exposing the stable core described in PROJECT_PLAN.md.
//
// As of v0.4.0 (rebased onto v0.5.0's read-only flow/result backfill),
// exactly 20 tools are registered: the 14 read-only tools from
// v0.1.0-v0.5.0 (velo_health_check, velo_search_clients,
// velo_get_client_info, velo_list_artifact_names,
// velo_get_artifact_details, velo_list_dfir_profiles,
// velo_get_dfir_profile, velo_validate_dfir_profile,
// velo_plan_dfir_triage, velo_compare_dfir_profiles,
// velo_find_profiles_by_artifact, velo_list_flows, velo_get_flow_status,
// velo_get_flow_results), plus six new tools implementing this
// project's first write-capable Velociraptor operations: a controlled,
// auditable, single-client collection pilot.
//
//   - velo_collect_artifact_with_approval / velo_collect_dfir_profile_with_approval /
//     velo_cancel_flow_with_approval (tools_collection.go): approval-gated
//     writes, each requiring case_id, reason, requester, target, and an
//     approval_reference that must resolve to an approved, unconsumed,
//     unexpired, fingerprint-matching approval.Store record.
//   - velo_list_flow_uploads / velo_get_flow_upload_metadata
//     (tools_flows.go): read-only, no approval required.
//   - velo_download_flow_upload_with_approval (tools_flows.go):
//     approval-gated like the collection tools; writes evidence bytes to
//     a local, operator-configured directory and returns metadata only,
//     never raw bytes inline.
//
// Every approval-gated tool is disabled by default (see
// writePilotEnabled) unless both policy.mode is "controlled" and
// approval.store_path is configured. No MCP tool can create or decide an
// approval.Request — only the agentic-velociraptor-mcp `approve` CLI
// subcommand can, so no MCP client (including an LLM driving one) can
// self-approve its own request. This is a controlled pilot, not
// unrestricted Velociraptor write access: no hunts, no multi-client
// collection, no raw VQL, and no destructive action exist anywhere in
// this codebase. See docs/security-model.md and docs/approval-flow.md.
//
// The five visibility tools and the three flow/result tools call
// Deps.ReadClient for a real Velociraptor gRPC response when
// Deps.VelociraptorReadMode is "real", and honestly report mock mode
// with no Velociraptor call otherwise; the two read-only upload tools
// follow the same convention. The three workflow tools read only the
// already-loaded DFIR profile registry and local policy. Hunt
// management and raw VQL remain entirely out of scope; see
// PROJECT_PLAN.md.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/config"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/dfir"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/policy"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
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

	// ReadClient uses config.VelociraptorConfig.ReadAPIConfigPath. The
	// five visibility tools, the three flow/result tools, and the two
	// read-only upload tools call it when VelociraptorReadMode is
	// "real".
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
	// must only be invoked by handlers after writePilotEnabled reports
	// true and a matching approval has been verified and consumed. Used
	// by velo_collect_artifact_with_approval,
	// velo_collect_dfir_profile_with_approval,
	// velo_cancel_flow_with_approval, and
	// velo_download_flow_upload_with_approval.
	WriteClient velociraptor.Client

	// VelociraptorWriteMode mirrors VelociraptorReadMode for WriteClient:
	// "real" if velociraptor.write_api_config_path was configured and
	// loaded successfully, "mock" otherwise. A "mock" WriteClient still
	// participates in the approval-gating logic (an approved request
	// will be checked and consumed) but every underlying Velociraptor
	// call then fails honestly with velociraptor.ErrNotImplemented,
	// since this milestone's hand-authored veloapi proto mirror does not
	// yet wire the real collection/cancel/upload RPCs; see
	// docs/security-model.md's known limitations.
	VelociraptorWriteMode string
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
// implemented for the current release. v0.4.0 adds the controlled,
// approval-gated collection/cancel/download tool groups on top of
// v0.5.0's read-only flow/result backfill; hunt management and raw VQL
// remain deferred/out of scope in PROJECT_PLAN.md.
func New(name, version string, deps Deps) *Server {
	s := mcp.NewServer(&mcp.Implementation{Name: name, Version: version}, nil)

	registerVisibilityTools(s, deps)
	registerProfileTools(s, deps)
	registerWorkflowTools(s, deps)
	registerCollectionTools(s, deps)
	registerFlowTools(s, deps)

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
// TODO(v0.6.0): decide on a fallback (e.g. stderr line) if deps.Audit.Write
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

// readOnlyAnnotations returns the ToolAnnotations for a read-only,
// non-destructive, closed-world tool (it only ever looks at local
// config/registry state or a Velociraptor read call, never mutates
// endpoint or server state).
func readOnlyAnnotations(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    true,
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(false),
		IdempotentHint:  true,
	}
}

// writeAnnotations returns the ToolAnnotations for an approval-gated,
// write-capable tool. Every such tool in this codebase is non-destructive
// (it starts/cancels/downloads a flow, never deletes or corrupts data)
// and closed-world (it only ever talks to the configured Velociraptor
// server, never an open-ended external system), but is not read-only
// (it creates or mutates flow state) and is not idempotent (calling it
// twice, even with identical arguments, starts/cancels/downloads twice —
// and in practice cannot happen twice at all, since each call consumes
// its approval reference exactly once).
func writeAnnotations(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:           title,
		ReadOnlyHint:    false,
		DestructiveHint: boolPtr(false),
		OpenWorldHint:   boolPtr(false),
		IdempotentHint:  false,
	}
}

// writePilotEnabled reports whether the controlled write pilot is
// active, and if not, a human-readable reason. Both conditions below
// must hold; either one being unset is sufficient to keep every
// approval-gated tool refusing to do anything but report itself
// disabled — the pilot must never turn on merely because an operator
// forgot to unset one of two settings.
func writePilotEnabled(deps Deps) (bool, string) {
	if deps.Policy == nil || deps.Policy.ReadOnly() {
		return false, `the controlled write pilot is disabled: policy.mode must be "controlled" (default is "read_only")`
	}
	if deps.Approvals == nil {
		return false, "the controlled write pilot is disabled: approval.store_path is not configured"
	}
	return true, ""
}

// verifyAndConsumeApproval resolves reference against deps.Approvals and
// checks it authorizes exactly candidate: not merely "some approval
// exists," but an approval whose approval.RequestFingerprint matches
// candidate's targeting fields exactly (operation, case_id, client_id,
// artifact/profile, parameters, flow_id, upload_name as applicable). On
// any failure it returns a populated response.Result (NotFound if the
// reference itself doesn't exist, Error for every other rejection
// reason) and the audit.Outcome the caller should record; the caller
// must not proceed to call Velociraptor. On success it consumes the
// approval (single-use, called before the caller's Velociraptor call so
// a failed attempt still burns the approval) and returns ok=true.
func verifyAndConsumeApproval(ctx context.Context, deps Deps, reference string, candidate approval.Request) (result response.Result, outcome audit.Outcome, ok bool) {
	status, err := deps.Approvals.Get(ctx, reference)
	if errors.Is(err, approval.ErrRequestNotFound) {
		return response.NotFound(fmt.Sprintf("approval reference %q was not found", reference)), audit.OutcomeBlocked, false
	}
	if err != nil {
		return response.Error(fmt.Sprintf("approval store error: %v", err)), audit.OutcomeError, false
	}

	if approval.RequestFingerprint(status.Request) != approval.RequestFingerprint(candidate) {
		return response.Error("approval reference does not match this operation's case_id/client_id/artifact/profile/parameters/flow_id/upload_name"), audit.OutcomeBlocked, false
	}
	if status.Decision == nil {
		return response.Error("approval reference has not yet been decided; ask a human operator to approve or deny it via the approve CLI"), audit.OutcomeBlocked, false
	}
	if !status.Decision.Approved {
		msg := "approval reference was denied"
		if status.Decision.Note != "" {
			msg = fmt.Sprintf("%s: %s", msg, status.Decision.Note)
		}
		return response.Error(msg), audit.OutcomeBlocked, false
	}
	if status.Consumed {
		return response.Error("approval reference has already been used"), audit.OutcomeBlocked, false
	}
	if status.Expired {
		return response.Error("approval reference has expired"), audit.OutcomeBlocked, false
	}

	if err := deps.Approvals.Consume(ctx, reference); err != nil {
		return response.Error(fmt.Sprintf("failed to consume approval: %v", err)), audit.OutcomeError, false
	}
	return response.Result{}, audit.OutcomeSuccess, true
}
