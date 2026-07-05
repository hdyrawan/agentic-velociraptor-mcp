package policy

// DecisionResult is the verdict returned by an Engine evaluation method.
// It is kept separate from approval.Decision: this is the MCP-layer
// policy verdict (is this operation even eligible to proceed), whereas
// approval.Decision is the human's yes/no on a specific Request.
type DecisionResult string

const (
	// DecisionAllow: proceed without approval.
	DecisionAllow DecisionResult = "allow"

	// DecisionRequireApproval: eligible, but must go through
	// internal/approval before execution.
	DecisionRequireApproval DecisionResult = "require_approval"

	// DecisionDeny: not eligible under any circumstance (e.g. read-only
	// mode, artifact/profile not allowlisted, raw VQL, target-all
	// disabled).
	DecisionDeny DecisionResult = "deny"
)

// Decision is the result of evaluating a single tool call against
// policy, including the reason a reviewer would see in the audit log.
type Decision struct {
	Result DecisionResult
	Reason string
}

// Allow returns an allow Decision with a reason.
func Allow(reason string) Decision {
	return Decision{Result: DecisionAllow, Reason: reason}
}

// RequireApproval returns a require_approval Decision with a reason.
func RequireApproval(reason string) Decision {
	return Decision{Result: DecisionRequireApproval, Reason: reason}
}

// Deny returns a deny Decision with a reason.
func Deny(reason string) Decision {
	return Decision{Result: DecisionDeny, Reason: reason}
}
