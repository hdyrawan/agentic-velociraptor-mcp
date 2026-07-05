// Package approval defines the human-approval workflow that every
// endpoint-changing or evidence-disclosing operation must pass through:
// collections, hunts (start/cancel), flow cancellation, and upload
// download.
//
// The MCP server is not the security boundary by itself; approval is one
// of the MCP-layer controls that sit on top of Velociraptor-native ACLs.
// A tool that "requires approval" must refuse to call Velociraptor at
// all until a matching Decision with Approved=true exists for its
// Request.
package approval

import "time"

// Operation names a category of risky action. These strings are shared
// with config.PolicyConfig.RequireApprovalFor, so they must stay in
// sync; see internal/policy for the canonical list.
type Operation string

const (
	OperationCollectArtifact    Operation = "collect_artifact"
	OperationCollectDFIRProfile Operation = "collect_dfir_profile"
	OperationStartHunt          Operation = "start_hunt"
	OperationStartDFIRHunt      Operation = "start_dfir_hunt"
	OperationCancelFlow         Operation = "cancel_flow"
	OperationCancelHunt         Operation = "cancel_hunt"
	OperationDownloadFlowUpload Operation = "download_flow_upload"
)

// Request captures everything needed for a human to make an informed
// approve/deny decision, and everything needed to audit that decision
// later. It intentionally requires a CaseID and Reason: risky operations
// must be traceable to an investigation, not just to "the agent asked."
type Request struct {
	ID string `json:"id"`

	Operation Operation `json:"operation"`

	// CaseID ties this request to an investigation/case identifier.
	// Required: empty CaseID must be rejected by the approval store, not
	// silently accepted.
	CaseID string `json:"case_id"`

	// Reason is operator/agent-supplied justification for why this
	// operation is needed.
	Reason string `json:"reason"`

	// ClientID / Artifact / Profile / HuntID / FlowID describe the
	// concrete target. Only the fields relevant to Operation need be
	// set.
	ClientID string `json:"client_id,omitempty"`
	Artifact string `json:"artifact,omitempty"`
	Profile  string `json:"profile,omitempty"`
	HuntID   string `json:"hunt_id,omitempty"`
	FlowID   string `json:"flow_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
}

// Decision is the outcome of a Request, recorded independently of the
// request so that the audit trail shows who decided what and when.
//
// TODO(v0.2.0): decide how a Decision is actually produced. Candidates:
// an out-of-band approval channel (ticket system, chat approval bot) or
// a local operator-confirmation prompt. Whatever mechanism is chosen
// must not let the same MCP client that requested the operation also
// self-approve it, or "approval" is theater.
type Decision struct {
	RequestID string `json:"request_id"`

	Approved bool `json:"approved"`

	// ApprovedBy identifies the human/system that made the decision.
	// Never the requesting agent/session.
	ApprovedBy string `json:"approved_by"`

	DecidedAt time.Time `json:"decided_at"`

	// Note is an optional human note (denial reason, conditions, etc.).
	Note string `json:"note,omitempty"`
}
