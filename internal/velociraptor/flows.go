package velociraptor

import "context"

// FlowState mirrors Velociraptor's own flow state machine at the level
// of detail tool responses need.
type FlowState string

const (
	FlowStateRunning   FlowState = "running"
	FlowStateFinished  FlowState = "finished"
	FlowStateError     FlowState = "error"
	FlowStateCancelled FlowState = "cancelled"
)

// FlowSummary describes one collection flow.
type FlowSummary struct {
	FlowID    string
	ClientID  string
	Artifact  string
	State     FlowState
	CreatedAt string
}

// FlowResultPage is a bounded page of rows from a flow's results,
// already truncated to policy limits (config.VelociraptorConfig.MaxRows
// / MaxResultBytes) by the implementation. Truncated is set when more
// rows existed than were returned, so tool responses can say so
// explicitly rather than implying completeness.
type FlowResultPage struct {
	Rows         []map[string]any
	Truncated    bool
	TotalRows    int
	ReturnedRows int
}

// FlowReader backs velo_list_flows, velo_get_flow_status, and
// velo_get_flow_results. These are read-only and use the read API
// identity.
type FlowReader interface {
	ListFlows(ctx context.Context, clientID string, limit int) ([]FlowSummary, error)
	GetFlowStatus(ctx context.Context, clientID, flowID string) (FlowSummary, error)
	GetFlowResults(ctx context.Context, clientID, flowID string, maxRows int, maxBytes int64) (FlowResultPage, error)
}

func (placeholderClient) ListFlows(ctx context.Context, clientID string, limit int) ([]FlowSummary, error) {
	return nil, ErrNotImplemented
}

func (placeholderClient) GetFlowStatus(ctx context.Context, clientID, flowID string) (FlowSummary, error) {
	return FlowSummary{}, ErrNotImplemented
}

func (placeholderClient) GetFlowResults(ctx context.Context, clientID, flowID string, maxRows int, maxBytes int64) (FlowResultPage, error) {
	return FlowResultPage{}, ErrNotImplemented
}

// CollectionRequest describes a single-artifact collection against one
// client. Parameters must already be produced via
// internal/vql.RenderParams (safe parameter binding), never raw VQL
// text.
type CollectionRequest struct {
	ClientID   string
	Artifact   string
	Parameters map[string]string
}

// FlowWriter backs velo_collect_artifact_with_approval,
// velo_collect_dfir_profile_with_approval, and
// velo_cancel_flow_with_approval. Every method here must only be called
// after a tool handler has confirmed an approval.Decision authorizes it,
// and must use the write API identity, never the read identity.
type FlowWriter interface {
	CollectArtifact(ctx context.Context, req CollectionRequest) (FlowSummary, error)
	CancelFlow(ctx context.Context, clientID, flowID string) error
}

func (placeholderClient) CollectArtifact(ctx context.Context, req CollectionRequest) (FlowSummary, error) {
	return FlowSummary{}, ErrNotImplemented
}

func (placeholderClient) CancelFlow(ctx context.Context, clientID, flowID string) error {
	return ErrNotImplemented
}
