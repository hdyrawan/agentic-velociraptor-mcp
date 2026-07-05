package approval

import "context"

// Store persists Requests and their eventual Decisions, and is the
// single place a tool handler consults to answer "is this operation
// currently authorized?"
//
// TODO(v0.2.0): implement a real Store. It must, at minimum:
//   - reject Requests with empty CaseID or Reason
//   - make Decision.ApprovedBy verifiable as distinct from the
//     requesting MCP session/identity
//   - expire unused approvals after a short TTL so a stale approval
//     can't be replayed against an unrelated later call
//   - be safe for concurrent use
//
// No implementation is provided yet; this interface only fixes the
// shape so tool-handler code written against it in v0.1.0+ does not need
// to change when the real store lands.
type Store interface {
	// Create records a new Request awaiting decision and returns it
	// (typically with ID populated).
	Create(ctx context.Context, req Request) (Request, error)

	// Decide records a Decision for a previously created Request.
	Decide(ctx context.Context, dec Decision) error

	// Get returns the Request and, if decided, its Decision.
	Get(ctx context.Context, requestID string) (Request, *Decision, error)

	// IsApproved reports whether requestID exists and has an approved,
	// not-yet-consumed Decision. Implementations should ensure an
	// approval can only authorize a single execution (consume-on-use),
	// so a tool handler cannot loop a single human approval into
	// unlimited operations.
	IsApproved(ctx context.Context, requestID string) (bool, error)
}
