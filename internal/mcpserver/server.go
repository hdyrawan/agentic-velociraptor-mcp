// Package mcpserver wires the Velociraptor-facing packages
// (config, policy, approval, audit, dfir, velociraptor, vql) into an MCP
// server exposing the stable core described in PROJECT_PLAN.md.
//
// As of v0.8.0 (preserving v0.7.0's completed stable-core inventory),
// exactly 28 tools are registered: the 14 read-only tools
// from v0.1.0-v0.5.0 (velo_health_check, velo_search_clients,
// velo_get_client_info, velo_list_artifact_names,
// velo_get_artifact_details, velo_list_dfir_profiles,
// velo_get_dfir_profile, velo_validate_dfir_profile,
// velo_plan_dfir_triage, velo_compare_dfir_profiles,
// velo_find_profiles_by_artifact, velo_list_flows, velo_get_flow_status,
// velo_get_flow_results), plus six v0.4.0 collection/evidence tools
// (velo_collect_artifact_with_approval,
// velo_collect_dfir_profile_with_approval, velo_cancel_flow_with_approval,
// velo_list_flow_uploads, velo_get_flow_upload_metadata,
// velo_download_flow_upload_with_approval), plus 7 v0.6.0 hunt management
// tools (velo_preview_hunt_scope, velo_start_hunt_with_approval,
// velo_start_dfir_hunt_with_approval, velo_list_hunts,
// velo_get_hunt_status, velo_get_hunt_results,
// velo_cancel_hunt_with_approval), plus the one v0.7.0 IOC helper tool
// (velo_hunt_ioc_with_approval).
//
// The six v0.4.0 collection/evidence tools implement this project's first
// write-capable Velociraptor operations: a controlled, auditable,
// single-client collection pilot. The seven v0.6.0 hunt management tools
// add approval-gated hunt start (single-artifact and DFIR-profile) and
// cancel, plus read-only scout/preview, list, status, and results.
// v0.7.0's velo_hunt_ioc_with_approval reuses that same approval/scope/
// audit machinery to hunt one validated indicator (hash, IP, domain,
// process, or path) through a fixed, allowlisted internal/vql template —
// never a caller-chosen artifact, never raw VQL.
//
// Every approval-gated tool is disabled by default (see
// writePilotEnabled) unless both policy.mode is "controlled" and
// approval.store_path is configured. No MCP tool can create or decide an
// approval.Request — only the agentic-velociraptor-mcp `approve` CLI
// subcommand can, so no MCP client (including an LLM driving one) can
// self-approve its own request. This is a controlled pilot, not
// unrestricted Velociraptor write access: no raw VQL, no unrestricted
// hunt creation, and no destructive action exist anywhere in this
// codebase. See docs/security-model.md and docs/approval-flow.md.
//
// The visibility tools, flow/result tools, and read-only hunt tools
// (preview, list, status, results) call Deps.ReadClient for a real
// Velociraptor gRPC response when Deps.VelociraptorReadMode is "real",
// and honestly report mock mode with no Velociraptor call otherwise;
// the two read-only upload tools follow the same convention. The three
// workflow tools read only the already-loaded DFIR profile registry and
// local policy. See PROJECT_PLAN.md.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
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
	// visibility tools, flow/result tools, read-only upload tools, and
	// read-only hunt tools (preview, list, status, results) call it when
	// VelociraptorReadMode is
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
	// velo_cancel_flow_with_approval,
	// velo_download_flow_upload_with_approval,
	// velo_start_hunt_with_approval,
	// velo_start_dfir_hunt_with_approval,
	// velo_cancel_hunt_with_approval, and
	// velo_hunt_ioc_with_approval.
	WriteClient velociraptor.Client

	// VelociraptorWriteMode mirrors VelociraptorReadMode for WriteClient:
	// "real" if velociraptor.write_api_config_path was configured and
	// loaded successfully, "mock" otherwise. A "mock" WriteClient still
	// participates in the approval-gating logic (an approved request
	// will be checked and consumed) but every underlying Velociraptor
	// call then fails honestly with velociraptor.ErrNotImplemented,
	// since this milestone's hand-authored veloapi proto mirror does not
	// yet wire the real collection/cancel/upload/hunt RPCs; see
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
// implemented for the current release. v0.8.0 keeps the v0.7.0 28-tool
// inventory unchanged while tightening backend-capability checks before
// approval consumption.
func New(name, version string, deps Deps) *Server {
	s := mcp.NewServer(&mcp.Implementation{Name: name, Version: version}, nil)

	registerVisibilityTools(s, deps)
	registerProfileTools(s, deps)
	registerWorkflowTools(s, deps)
	registerCollectionTools(s, deps)
	registerFlowTools(s, deps)
	registerHuntTools(s, deps)
	registerIOCTools(s, deps)

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
// did), but they are no longer silent either: a failed write falls back
// to a structured stderr line so a misconfigured audit path is
// discoverable at operation time rather than only via missing log
// entries. Approval-gated write handlers additionally fail closed on a
// broken sink before executing anything; see gateAuditForWrite.
//
// Reason and RequestReason are routed through audit.Sanitizer before
// being written. These fields routinely carry err.Error() strings that
// may have interpolated user input or, worse, PEM-shaped key material
// from a gRPC/TLS error. SanitizeString excises any embedded PEM block
// so the audit log can never accidentally persist a leaked secret
// hidden inside an innocent-looking error message. Key-name based
// redaction of structured fields happens in the caller before the
// audit.Event is built (see docs/security-model.md).
func recordAudit(deps Deps, evt audit.Event) {
	if deps.Audit == nil {
		return
	}
	evt.Timestamp = time.Now().UTC()
	if deps.Config != nil {
		sanitizer := audit.NewSanitizer(deps.Config.Audit.RedactFields)
		evt.Reason = sanitizer.SanitizeString(evt.Reason)
		evt.RequestReason = sanitizer.SanitizeString(evt.RequestReason)
	}
	if err := deps.Audit.Write(evt); err != nil {
		fmt.Fprintf(os.Stderr, "agentic-velociraptor-mcp: audit write failed (tool=%s outcome=%s): %v\n", evt.Tool, evt.Outcome, err)
	}
}

// gateAuditForWrite durably records the pre-execution audit.OutcomeAttempt
// event for an approval-gated write operation. Handlers call it after
// every policy/approval/backend gate has passed and immediately before
// consumeApproval: if the event cannot be persisted, the operation is
// refused (fail closed) with the single-use approval left unconsumed,
// so an endpoint-facing write can never execute without an audit record
// of the attempt. The returned Result is populated only on failure.
func gateAuditForWrite(deps Deps, evt audit.Event) (response.Result, bool) {
	if deps.Audit == nil {
		return response.Result{}, true
	}
	evt.Timestamp = time.Now().UTC()
	evt.Outcome = audit.OutcomeAttempt
	if deps.Config != nil {
		sanitizer := audit.NewSanitizer(deps.Config.Audit.RedactFields)
		evt.Reason = sanitizer.SanitizeString(evt.Reason)
		evt.RequestReason = sanitizer.SanitizeString(evt.RequestReason)
	}
	if err := deps.Audit.Write(evt); err != nil {
		fmt.Fprintf(os.Stderr, "agentic-velociraptor-mcp: audit write failed (tool=%s outcome=%s): %v\n", evt.Tool, evt.Outcome, err)
		return response.Error("audit log is unavailable; refusing to execute this approval-gated operation (the approval was not consumed)"), false
	}
	return response.Result{}, true
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

// backendOperationReady checks whether the concrete write backend is
// present and advertises support for a typed operation before a
// single-use approval is consumed. Fakes that do not implement
// velociraptor.BackendOperationSupporter are treated as capable so tests
// can continue to exercise handler behaviour through direct method
// overrides; shipped placeholder/grpc clients implement the interface and
// return false for scaffolded operations.
func backendOperationReady(client velociraptor.Client, operation velociraptor.BackendOperation) response.Result {
	if client == nil {
		return response.Error("real mode is configured but no Velociraptor write client is available")
	}
	if supporter, ok := client.(velociraptor.BackendOperationSupporter); ok && !supporter.SupportsBackendOperation(operation) {
		return response.Error(fmt.Sprintf("backend_not_implemented: Velociraptor backend operation %q is not implemented by this build", operation))
	}
	return response.Result{}
}

// verifyApproval resolves reference against deps.Approvals and checks it
// authorizes exactly candidate: not merely "some approval exists," but
// an approval whose approval.RequestFingerprint matches candidate's
// targeting fields exactly (operation, case_id, client_id,
// artifact/profile, parameters, flow_id, upload_name as applicable). It
// deliberately does not consume the approval: handlers must run
// backend-capability gates after this check and before consumeApproval.
func verifyApproval(ctx context.Context, deps Deps, reference string, candidate approval.Request) (result response.Result, outcome audit.Outcome, ok bool) {
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

	return response.Result{}, audit.OutcomeSuccess, true
}

// consumeApproval burns a verified approval reference immediately before
// the handler calls a ready Velociraptor backend. This sequencing keeps
// approval single-use for real execution attempts while preserving
// approvals when policy/input/scope/allowlist/backend gates fail.
func consumeApproval(ctx context.Context, deps Deps, reference string) (response.Result, audit.Outcome, bool) {
	if err := deps.Approvals.Consume(ctx, reference); err != nil {
		return response.Error(fmt.Sprintf("failed to consume approval: %v", err)), audit.OutcomeError, false
	}
	return response.Result{}, audit.OutcomeSuccess, true
}
