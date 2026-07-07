package velociraptor

import (
	"context"
	"errors"
)

// ErrHuntNotFound is returned by GetHuntStatus and GetHuntResults when
// the requested hunt does not exist.
var ErrHuntNotFound = errors.New("velociraptor: hunt not found")

// ErrTargetAllDisabled is returned by PreviewHuntScope when all-clients
// targeting is requested but policy disallows it.
var ErrTargetAllDisabled = errors.New("velociraptor: targeting all clients is disabled by policy")

// HuntState mirrors Velociraptor hunt states at the level tool
// responses need.
type HuntState string

const (
	HuntStateRunning HuntState = "running"
	HuntStateStopped HuntState = "stopped"
	HuntStatePaused  HuntState = "paused"
)

// HuntSummary describes one hunt.
type HuntSummary struct {
	HuntID      string
	Artifact    string
	State       HuntState
	CreatedAt   string
	ClientCount int
}

// HuntScopePreview reports how many/which clients a proposed hunt scope
// would match, without creating the hunt. Every hunt start must be
// preceded by a preview so an approver sees blast radius before
// approving; see velo_preview_hunt_scope.
type HuntScopePreview struct {
	MatchedClientCount int
	SampleClientIDs    []string
	ExceedsMaxClients  bool
}

// HuntReader backs velo_list_hunts, velo_get_hunt_status,
// velo_get_hunt_results, and velo_preview_hunt_scope.
type HuntReader interface {
	ListHunts(ctx context.Context, limit int) ([]HuntSummary, error)
	GetHuntStatus(ctx context.Context, huntID string) (HuntSummary, error)
	// GetHuntResults returns one page of the hunt's results for its
	// artifact. source is optional; see FlowReader.GetFlowResults's doc
	// comment — the same named-source disambiguation applies here.
	GetHuntResults(ctx context.Context, huntID, source string, maxRows int, maxBytes int64) (FlowResultPage, error)

	// PreviewHuntScope resolves a scope (see validation.HuntScope)
	// against the current client population without starting anything.
	PreviewHuntScope(ctx context.Context, scope HuntScopeRequest) (HuntScopePreview, error)
}

func (placeholderClient) ListHunts(ctx context.Context, limit int) ([]HuntSummary, error) {
	return nil, ErrNotImplemented
}

func (placeholderClient) GetHuntStatus(ctx context.Context, huntID string) (HuntSummary, error) {
	return HuntSummary{}, ErrNotImplemented
}

func (placeholderClient) GetHuntResults(ctx context.Context, huntID, source string, maxRows int, maxBytes int64) (FlowResultPage, error) {
	return FlowResultPage{}, ErrNotImplemented
}

func (placeholderClient) PreviewHuntScope(ctx context.Context, scope HuntScopeRequest) (HuntScopePreview, error) {
	return HuntScopePreview{}, ErrNotImplemented
}

// HuntScopeRequest is the gRPC-layer shape corresponding to
// validation.HuntScope, kept as a separate type so internal/velociraptor
// does not need to import internal/validation.
type HuntScopeRequest struct {
	ClientIDs []string
	Label     string
	All       bool
}

// HuntRequest describes a hunt to start, either a single artifact or (in
// the DFIR hunt case) a profile expanded to its constituent artifacts by
// the caller before reaching this layer.
type HuntRequest struct {
	Artifact   string
	Parameters map[string]string
	Scope      HuntScopeRequest
	MaxClients int
}

// HuntWriter backs velo_start_hunt_with_approval,
// velo_start_dfir_hunt_with_approval, and velo_cancel_hunt_with_approval.
// As with FlowWriter, every method here requires a prior approval
// decision and uses the write API identity.
type HuntWriter interface {
	StartHunt(ctx context.Context, req HuntRequest) (HuntSummary, error)
	CancelHunt(ctx context.Context, huntID string) error
}

func (placeholderClient) StartHunt(ctx context.Context, req HuntRequest) (HuntSummary, error) {
	return HuntSummary{}, ErrNotImplemented
}

func (placeholderClient) CancelHunt(ctx context.Context, huntID string) error {
	return ErrNotImplemented
}
