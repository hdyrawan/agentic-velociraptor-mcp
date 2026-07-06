package approval

import (
	"context"
	"errors"
)

var (
	// ErrRequestNotFound is returned by Get/Decide/IsApproved/Consume
	// when no Request with the given reference exists in the Store.
	ErrRequestNotFound = errors.New("approval: request not found")

	// ErrReferenceRequired is returned by Create when req.ID (the
	// human-chosen approval reference) is empty.
	ErrReferenceRequired = errors.New("approval: reference (request id) is required")

	// ErrDuplicateReference is returned by Create when req.ID already
	// exists in the Store: references are never reused or overwritten.
	ErrDuplicateReference = errors.New("approval: reference already exists")

	// ErrCaseIDRequired is returned by Create when req.CaseID is empty.
	ErrCaseIDRequired = errors.New("approval: case_id is required")

	// ErrReasonRequired is returned by Create when req.Reason is empty.
	ErrReasonRequired = errors.New("approval: reason is required")

	// ErrRequesterRequired is returned by Create when req.Requester is
	// empty.
	ErrRequesterRequired = errors.New("approval: requester is required")

	// ErrApprovedByRequired is returned by Decide when dec.ApprovedBy is
	// empty.
	ErrApprovedByRequired = errors.New("approval: approved_by is required")

	// ErrAlreadyDecided is returned by Decide when the referenced
	// Request already has a Decision: decisions are immutable once made.
	ErrAlreadyDecided = errors.New("approval: request has already been decided")

	// ErrAlreadyConsumed is returned by Consume when the referenced
	// Request's approval has already been used once.
	ErrAlreadyConsumed = errors.New("approval: approval has already been consumed")
)

// Status is the full lifecycle state of one Request: the request itself,
// its Decision if any, whether it has already been consumed by a prior
// execution, and whether its approval window has expired (Store
// implementations decide expiry, typically relative to Request.CreatedAt
// or Decision.DecidedAt and a configured TTL).
type Status struct {
	Request  Request
	Decision *Decision
	Consumed bool
	Expired  bool
}

// Store persists Requests and their eventual Decisions, and is the
// single place a tool handler consults to answer "is this operation
// currently authorized?"
//
// A Store implementation must, at minimum:
//   - reject Requests with empty ID (reference), CaseID, Reason, or
//     Requester
//   - reject a duplicate reference rather than overwriting an existing
//     Request
//   - reject Decisions with empty ApprovedBy, and reject deciding a
//     Request twice
//   - expire unused approvals after a configured TTL so a stale approval
//     can't be replayed against an unrelated later call
//   - make an approval usable exactly once (see Consume)
//   - be safe for concurrent use
//
// No tool handler in internal/mcpserver ever calls Create or Decide:
// those are only reachable from the agentic-velociraptor-mcp `approve`
// CLI subcommand, which is not part of the MCP tool surface. This is
// what makes "approval" meaningful rather than theater: the MCP client
// driving tool calls can request that something be done (by supplying a
// reference to a human) but can never grant itself permission to do it.
type Store interface {
	// Create records a new Request awaiting decision and returns it. The
	// caller must set req.ID to the desired approval reference; Create
	// does not generate one.
	Create(ctx context.Context, req Request) (Request, error)

	// Decide records a Decision for a previously created Request.
	Decide(ctx context.Context, dec Decision) error

	// Get returns the full lifecycle Status for requestID, or
	// ErrRequestNotFound if no such reference exists.
	Get(ctx context.Context, requestID string) (Status, error)

	// IsApproved reports whether requestID exists and currently has an
	// approved, unconsumed, unexpired Decision. A missing reference
	// reports (false, nil), not an error: "is this approved" is a
	// boolean question a caller can ask before deciding how to react to
	// "no."
	IsApproved(ctx context.Context, requestID string) (bool, error)

	// Consume marks requestID's approval as used. It is an error to
	// consume an already-consumed or nonexistent reference. Tool
	// handlers must call Consume before invoking the corresponding
	// Velociraptor operation (not after), so a single human approval can
	// never authorize more than one execution attempt regardless of
	// whether that attempt succeeds.
	Consume(ctx context.Context, requestID string) error
}
