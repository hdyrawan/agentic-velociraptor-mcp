package velociraptor

import (
	"context"
	"errors"
	"fmt"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor/veloapi"
)

// ErrHuntScopeClientIDsUnsupported is returned by PreviewHuntScope and
// StartHunt when the requested scope is an explicit client-ID list.
//
// Real Velociraptor's Hunt/HuntCondition proto (api/proto/hunts.proto
// upstream) supports exactly two targeting modes: a set of labels
// (HuntCondition.labels) or an operating system
// (HuntCondition.os) — plus "no condition" meaning all clients. There is
// no field anywhere in that message for "this explicit list of client
// IDs"; CreateHunt/EstimateHunt have no way to schedule a hunt against
// only a caller-chosen ID list. This project's client_ids hunt-scope
// input (validation.HuntScope.ClientIDs) therefore has no safe typed
// Velociraptor RPC backing it in real mode. A workaround exists upstream
// (label the target clients via LabelClients, then run a label-scoped
// hunt, then unlabel them) but that mutates client state beyond what a
// hunt-start operation should do and was judged out of scope for this
// milestone without live-lab validation; see PROJECT_STATE.md's v0.9.0
// entry and docs/security-model.md.
//
// Label- and all-clients-scoped hunts are fully implemented against the
// real RPCs; only the explicit client_ids scope is blocked.
var ErrHuntScopeClientIDsUnsupported = errors.New("velociraptor: explicit client_ids hunt scope has no typed Velociraptor RPC support in real mode (hunts can only be scoped by label or all-clients); use label or all instead")

// huntConditionFromScope translates this project's HuntScopeRequest into
// a real Velociraptor HuntCondition, or returns
// ErrHuntScopeClientIDsUnsupported for the one scope mode Velociraptor's
// hunt proto cannot express. validation.ValidateHuntScope guarantees
// exactly one of ClientIDs/Label/All is set before this is ever called.
func huntConditionFromScope(scope HuntScopeRequest) (*veloapi.HuntCondition, error) {
	switch {
	case len(scope.ClientIDs) > 0:
		return nil, ErrHuntScopeClientIDsUnsupported
	case scope.Label != "":
		return &veloapi.HuntCondition{
			UnionField: &veloapi.HuntCondition_Labels{
				Labels: &veloapi.HuntLabelCondition{Label: []string{scope.Label}},
			},
		}, nil
	default:
		// All (or an empty scope, which validation prevents from
		// reaching a write path): nil condition means "every client" to
		// EstimateHunt/CreateHunt.
		return nil, nil
	}
}

// mapHuntState translates Velociraptor's real Hunt.State enum into this
// project's HuntState. ARCHIVED/DELETED/UNSET all collapse to "stopped"
// from this project's perspective: none of them are a hunt a caller
// could still be waiting on or need to cancel.
func mapHuntState(s veloapi.Hunt_State) HuntState {
	switch s {
	case veloapi.Hunt_RUNNING:
		return HuntStateRunning
	case veloapi.Hunt_PAUSED:
		return HuntStatePaused
	default:
		return HuntStateStopped
	}
}

// firstHuntArtifact returns the first artifact name a Hunt targets,
// preferring the top-level Artifacts list (which Velociraptor's hunt
// dispatcher populates from the start request) and falling back to the
// start request's Specs.
func firstHuntArtifact(h *veloapi.Hunt) string {
	if h == nil {
		return ""
	}
	if artifacts := h.GetArtifacts(); len(artifacts) > 0 {
		return artifacts[0]
	}
	return firstArtifactName(h.GetStartRequest())
}

func toHuntSummary(h *veloapi.Hunt) HuntSummary {
	return HuntSummary{
		HuntID:      h.GetHuntId(),
		Artifact:    firstHuntArtifact(h),
		State:       mapHuntState(h.GetState()),
		CreatedAt:   microsecondsToRFC3339(h.GetCreateTime()),
		ClientCount: int(h.GetStats().GetTotalClientsScheduled()),
	}
}

// ListHunts calls Velociraptor's typed ListHunts RPC (deprecated in
// favor of the GUI's GetHuntTable, but still implemented server-side, and
// unlike GetHuntTable it returns fully typed Hunt messages rather than
// JSON-encoded table rows — preferred here for that reason).
func (c *grpcClient) ListHunts(ctx context.Context, limit int) ([]HuntSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	bounded := boundLimit(limit, c.effectiveMaxRows())
	resp, err := c.hunts.ListHunts(ctx, &veloapi.ListHuntsRequest{Count: uint64(bounded)})
	if err != nil {
		return nil, fmt.Errorf("velociraptor: list hunts: %w", sanitizeTLSError(err))
	}

	items := resp.GetItems()
	if len(items) > bounded {
		items = items[:bounded]
	}
	out := make([]HuntSummary, 0, len(items))
	for _, h := range items {
		out = append(out, toHuntSummary(h))
	}
	return out, nil
}

// GetHuntStatus calls Velociraptor's GetHunt RPC. Unlike GetClient,
// upstream's hunt_dispatcher.GetHunt reports an unknown hunt ID as a real
// gRPC error (services.HuntNotFoundError) rather than a zero-value
// response, so isNotFoundError's substring heuristic is the primary
// detection path here; the empty-HuntId check is a defensive fallback.
func (c *grpcClient) GetHuntStatus(ctx context.Context, huntID string) (HuntSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.hunts.GetHunt(ctx, &veloapi.GetHuntRequest{HuntId: huntID})
	if err != nil {
		if isNotFoundError(err) {
			return HuntSummary{}, fmt.Errorf("velociraptor: get hunt status: %w", ErrHuntNotFound)
		}
		return HuntSummary{}, fmt.Errorf("velociraptor: get hunt status: %w", sanitizeTLSError(err))
	}
	if resp.GetHuntId() == "" {
		return HuntSummary{}, fmt.Errorf("velociraptor: get hunt status: %w", ErrHuntNotFound)
	}

	return toHuntSummary(resp), nil
}

// GetHuntResults first calls GetHunt to discover the hunt's artifact
// (GetHuntResultsRequest requires one), then calls Velociraptor's
// GetHuntResults RPC for that artifact's rows. Note that upstream's
// current GetHuntResults implementation hardcodes a 100-row server-side
// limit regardless of the requested Count (see api/hunts.go's
// RunVQL(... "LIMIT 100")); Offset/Count are still sent per the proto
// contract, but a real server may cap results at 100 rows independent of
// what this project asks for.
func (c *grpcClient) GetHuntResults(ctx context.Context, huntID string, maxRows int, maxBytes int64) (FlowResultPage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	hunt, err := c.hunts.GetHunt(ctx, &veloapi.GetHuntRequest{HuntId: huntID})
	if err != nil {
		if isNotFoundError(err) {
			return FlowResultPage{}, fmt.Errorf("velociraptor: get hunt results: %w", ErrHuntNotFound)
		}
		return FlowResultPage{}, fmt.Errorf("velociraptor: get hunt results: %w", sanitizeTLSError(err))
	}
	if hunt.GetHuntId() == "" {
		return FlowResultPage{}, fmt.Errorf("velociraptor: get hunt results: %w", ErrHuntNotFound)
	}

	artifact := firstHuntArtifact(hunt)
	if artifact == "" {
		return FlowResultPage{}, nil
	}

	bounded := boundLimit(maxRows, c.effectiveMaxRows())
	resp, err := c.hunts.GetHuntResults(ctx, &veloapi.GetHuntResultsRequest{
		HuntId:   huntID,
		Artifact: artifact,
		Count:    uint64(bounded),
	})
	if err != nil {
		return FlowResultPage{}, fmt.Errorf("velociraptor: get hunt results: %w", sanitizeTLSError(err))
	}

	rows, err := decodeTableRows(resp)
	if err != nil {
		return FlowResultPage{}, fmt.Errorf("velociraptor: get hunt results: %w", err)
	}

	total := int(resp.GetTotalRows())
	truncated := len(rows) >= bounded && (total == 0 || len(rows) < total)
	page := FlowResultPage{Rows: rows, TotalRows: total, ReturnedRows: len(rows), Truncated: truncated}
	if truncated {
		page.NextCursor = offsetCursor(len(rows))
	}
	return page, nil
}

// PreviewHuntScope calls Velociraptor's EstimateHunt RPC. It shares
// huntConditionFromScope with StartHunt so an explicit client_ids scope
// is refused consistently at both the preview and start steps, rather
// than a preview reporting a count for a scope StartHunt cannot actually
// enact.
//
// EstimateHunt's real response (HuntStats) carries only a count
// (TotalClientsScheduled), never a client-ID sample, so
// HuntScopePreview.SampleClientIDs is always empty for a real preview —
// an intentional, honest gap (see Info.ServerVersion's similar real-mode
// gap in HealthCheck), not a fabricated list.
func (c *grpcClient) PreviewHuntScope(ctx context.Context, scope HuntScopeRequest) (HuntScopePreview, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cond, err := huntConditionFromScope(scope)
	if err != nil {
		return HuntScopePreview{}, fmt.Errorf("velociraptor: preview hunt scope: %w", err)
	}

	resp, err := c.hunts.EstimateHunt(ctx, &veloapi.HuntEstimateRequest{Condition: cond})
	if err != nil {
		return HuntScopePreview{}, fmt.Errorf("velociraptor: preview hunt scope: %w", sanitizeTLSError(err))
	}

	return HuntScopePreview{MatchedClientCount: int(resp.GetTotalClientsScheduled())}, nil
}

// StartHunt calls Velociraptor's CreateHunt RPC with State=RUNNING (the
// hunt begins scheduling clients immediately on creation; this project
// never creates a hunt in the PAUSED state and starts it separately).
// See huntConditionFromScope for the one scope mode
// (explicit client_ids) this refuses in real mode.
func (c *grpcClient) StartHunt(ctx context.Context, req HuntRequest) (HuntSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cond, err := huntConditionFromScope(req.Scope)
	if err != nil {
		return HuntSummary{}, fmt.Errorf("velociraptor: start hunt: %w", err)
	}

	hunt := &veloapi.Hunt{
		HuntDescription: fmt.Sprintf("agentic-velociraptor-mcp: %s", req.Artifact),
		StartRequest:    artifactCollectorArgsFor("", req.Artifact, req.Parameters),
		Condition:       cond,
		ClientLimit:     uint64(req.MaxClients),
		State:           veloapi.Hunt_RUNNING,
	}

	// CreateHunt repurposes StartFlowResponse.FlowId to carry the newly
	// created HuntId (see api/hunts.go's CreateHunt upstream: "result.FlowId
	// = in.HuntId").
	resp, err := c.hunts.CreateHunt(ctx, hunt)
	if err != nil {
		return HuntSummary{}, fmt.Errorf("velociraptor: start hunt: %w", sanitizeTLSError(err))
	}

	return HuntSummary{
		HuntID:   resp.GetFlowId(),
		Artifact: req.Artifact,
		State:    HuntStateRunning,
	}, nil
}

// CancelHunt calls Velociraptor's ModifyHunt RPC to set the hunt's state
// to STOPPED; it never modifies a hunt's description, tags, expiry, or
// flow assignment.
func (c *grpcClient) CancelHunt(ctx context.Context, huntID string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, err := c.hunts.ModifyHunt(ctx, &veloapi.HuntMutation{HuntId: huntID, State: veloapi.Hunt_STOPPED})
	if err != nil {
		return fmt.Errorf("velociraptor: cancel hunt: %w", sanitizeTLSError(err))
	}
	return nil
}
