// Package audit defines the structured audit event emitted for every
// tool invocation and the JSONL sink it is written to.
//
// Every tool call must produce exactly one Event, regardless of outcome.
// This is a security control, not a debugging convenience: it must be
// possible to reconstruct, after the fact, exactly which Velociraptor
// operations an agent requested, what policy decision was made, and
// whether the operation actually ran.
package audit

import "time"

// Outcome is the terminal disposition of a tool invocation. These three
// values are exhaustive by design: every audited call ends in exactly
// one of them.
type Outcome string

const (
	// OutcomeSuccess: the operation was permitted and completed.
	OutcomeSuccess Outcome = "success"

	// OutcomeBlocked: the operation was denied by policy, approval, or
	// validation before it reached Velociraptor (or before it took
	// effect). Blocked is not an error: it means a control worked.
	OutcomeBlocked Outcome = "blocked"

	// OutcomeError: the operation was permitted but failed for an
	// operational reason (timeout, transport failure, malformed
	// response, etc.).
	OutcomeError Outcome = "error"
)

// Event is a single audit record. Fields map close to 1:1 with what a
// reviewer needs to answer "who asked for what, was it allowed, and what
// happened." Field values must already be redacted by the caller (see
// sanitize.go) before an Event is constructed; Event itself does not
// re-sanitize.
type Event struct {
	Timestamp time.Time `json:"timestamp"`

	// Tool is the MCP tool name, e.g. "velo_collect_artifact_with_approval".
	Tool string `json:"tool"`

	// Outcome is one of OutcomeSuccess, OutcomeBlocked, OutcomeError.
	Outcome Outcome `json:"outcome"`

	// Reason gives a short human-readable explanation, especially for
	// Blocked (which policy rule fired) and Error (what failed).
	Reason string `json:"reason,omitempty"`

	// ClientID is the Velociraptor client identifier the call targeted,
	// if any.
	ClientID string `json:"client_id,omitempty"`

	// FlowID / HuntID identify the flow or hunt the call targeted or
	// created, if any.
	FlowID string `json:"flow_id,omitempty"`
	HuntID string `json:"hunt_id,omitempty"`

	// Artifact / Profile identify the artifact or DFIR profile involved.
	Artifact string `json:"artifact,omitempty"`
	Profile  string `json:"profile,omitempty"`

	// IOCKind / IOCValue identify the indicator kind (hash, ip, domain,
	// process, path) and value velo_hunt_ioc_with_approval was asked to
	// hunt for. Neither is a secret (they are investigative indicator
	// data, the same category as ClientID/Artifact above), so both are
	// logged in plain text like every other targeting field.
	IOCKind  string `json:"ioc_kind,omitempty"`
	IOCValue string `json:"ioc_value,omitempty"`

	// CaseID / Reason (of the request, not the outcome) support
	// traceability of write-capable operations back to an
	// investigation. RequestReason is operator-supplied justification;
	// Reason above is system-supplied outcome justification.
	CaseID        string `json:"case_id,omitempty"`
	RequestReason string `json:"request_reason,omitempty"`

	// ApprovalID references the approval.Request that authorized this
	// call, if the tool required approval. Never the approval token
	// itself.
	ApprovalID string `json:"approval_id,omitempty"`

	// DurationMS is wall-clock time spent on the underlying Velociraptor
	// call(s), for latency and timeout auditing.
	DurationMS int64 `json:"duration_ms,omitempty"`

	// RowCount / ByteCount record how much data was returned, to make
	// result-limit enforcement independently verifiable from the log.
	RowCount  int   `json:"row_count,omitempty"`
	ByteCount int64 `json:"byte_count,omitempty"`
}
