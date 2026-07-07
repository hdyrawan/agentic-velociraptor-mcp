package velociraptor

import (
	"context"
	"fmt"
	"strings"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor/veloapi"
)

// mapFlowState translates Velociraptor's real ArtifactCollectorContext
// state (as its Stringer form, e.g. "RUNNING"/"FINISHED"/"ERROR") plus
// its free-text status message into this project's FlowState. Real
// Velociraptor has no separate "cancelled" state value — CancelFlow sets
// State=ERROR and prefixes Status with "Cancelled by <user>" (see
// launcher.CancelFlow upstream) — so that prefix is the only signal
// that distinguishes a cancelled flow from a genuinely failed one.
func mapFlowState(state, status string) FlowState {
	switch state {
	case "FINISHED":
		return FlowStateFinished
	case "ERROR":
		if strings.HasPrefix(status, "Cancelled by ") {
			return FlowStateCancelled
		}
		return FlowStateError
	case "RUNNING", "WAITING", "IN_PROGRESS", "UNRESPONSIVE":
		return FlowStateRunning
	default:
		return FlowStateRunning
	}
}

// firstArtifactName returns the first artifact name a
// ArtifactCollectorArgs targets, preferring the legacy Artifacts list
// (which this project always populates alongside Specs; see
// artifactCollectorArgsFor) and falling back to the first Specs entry
// for requests this project did not itself construct (e.g. a flow
// started via the Velociraptor GUI).
func firstArtifactName(req *veloapi.ArtifactCollectorArgs) string {
	if req == nil {
		return ""
	}
	if artifacts := req.GetArtifacts(); len(artifacts) > 0 {
		return artifacts[0]
	}
	if specs := req.GetSpecs(); len(specs) > 0 {
		return specs[0].GetArtifact()
	}
	return ""
}

// envFromParams converts a plain parameter map into VQLEnv key/value
// pairs, the only mechanism this project uses to bind a value into a
// collection or hunt request. Keys are iterated in the map's
// (unspecified) order; parameter order has no semantic meaning to
// Velociraptor, which matches env entries by key.
func envFromParams(params map[string]string) []*veloapi.VQLEnv {
	if len(params) == 0 {
		return nil
	}
	out := make([]*veloapi.VQLEnv, 0, len(params))
	for k, v := range params {
		out = append(out, &veloapi.VQLEnv{Key: k, Value: v})
	}
	return out
}

// artifactCollectorArgsFor builds the ArtifactCollectorArgs this project
// sends for a single-artifact collection or hunt start. Both Artifacts
// (the legacy name list some server-side table columns still read, e.g.
// GetClientFlows' "Artifacts" column) and Specs (which carries bound
// parameters) are populated, matching what Velociraptor's own web GUI
// sends when a user selects artifacts with parameters.
func artifactCollectorArgsFor(clientID, artifact string, params map[string]string) *veloapi.ArtifactCollectorArgs {
	return &veloapi.ArtifactCollectorArgs{
		ClientId:  clientID,
		Artifacts: []string{artifact},
		Specs: []*veloapi.ArtifactSpec{{
			Artifact: artifact,
			Parameters: &veloapi.ArtifactParameters{
				Env: envFromParams(params),
			},
		}},
	}
}

// toFlowSummaryFromContext builds a FlowSummary from a real
// ArtifactCollectorContext (the typed shape GetFlowDetails returns).
func toFlowSummaryFromContext(clientID string, fctx *veloapi.ArtifactCollectorContext) FlowSummary {
	return FlowSummary{
		FlowID:    fctx.GetSessionId(),
		ClientID:  clientID,
		Artifact:  firstArtifactName(fctx.GetRequest()),
		State:     mapFlowState(fctx.GetState().String(), fctx.GetStatus()),
		CreatedAt: microsecondsToRFC3339(fctx.GetCreateTime()),
	}
}

// toFlowSummaryFromRow builds a FlowSummary from one decoded
// GetClientFlows table row. Unlike toFlowSummaryFromContext, no Status
// text is available at this layer (GetClientFlows' table columns do not
// include it), so a cancelled flow is reported generically as "error"
// here; velo_get_flow_status (which calls GetFlowDetails, not this
// table) reports "cancelled" precisely.
func toFlowSummaryFromRow(clientID string, row map[string]any) FlowSummary {
	artifacts := rowStringSlice(row, "Artifacts")
	artifact := ""
	if len(artifacts) > 0 {
		artifact = artifacts[0]
	}
	return FlowSummary{
		FlowID:    rowStr(row, "FlowId"),
		ClientID:  clientID,
		Artifact:  artifact,
		State:     mapFlowState(rowStr(row, "State"), ""),
		CreatedAt: microsecondsToRFC3339(uint64(rowInt64(row, "Created"))),
	}
}

// ListFlows calls Velociraptor's GetClientFlows RPC (a GetTableRequest
// scoped by client_id), sorted by FlowId so the most recent flows sort
// first, matching the sort Velociraptor's own GUI applies.
func (c *grpcClient) ListFlows(ctx context.Context, clientID string, limit int, cursor string) ([]FlowSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	bounded := boundLimit(limit, c.effectiveMaxRows())
	startRow := parseOffsetCursor(cursor)

	resp, err := c.flows.GetClientFlows(ctx, &veloapi.GetTableRequest{
		ClientId:      clientID,
		Rows:          uint64(bounded),
		StartRow:      uint64(startRow),
		SortColumn:    "FlowId",
		SortDirection: true,
	})
	if err != nil {
		return nil, fmt.Errorf("velociraptor: list flows: %w", sanitizeTLSError(err))
	}

	rows, err := decodeTableRows(resp)
	if err != nil {
		return nil, fmt.Errorf("velociraptor: list flows: %w", err)
	}

	out := make([]FlowSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, toFlowSummaryFromRow(clientID, row))
	}
	return out, nil
}

// GetFlowStatus calls Velociraptor's GetFlowDetails RPC, a fully typed
// request/response pair (ApiFlowRequest -> FlowDetails) that needs no
// row decoding. An unknown flow_id is detected defensively (empty
// SessionId) the same way GetClientInfo detects an unknown client ID
// (see ErrClientNotFound), in case a future server version returns a
// zero-value context instead of an RPC error for this case; a
// gRPC-level error whose message mentions "not found" is also mapped to
// ErrFlowNotFound.
func (c *grpcClient) GetFlowStatus(ctx context.Context, clientID, flowID string) (FlowSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.flows.GetFlowDetails(ctx, &veloapi.ApiFlowRequest{ClientId: clientID, FlowId: flowID})
	if err != nil {
		if isNotFoundError(err) {
			return FlowSummary{}, fmt.Errorf("velociraptor: get flow status: %w", ErrFlowNotFound)
		}
		return FlowSummary{}, fmt.Errorf("velociraptor: get flow status: %w", sanitizeTLSError(err))
	}

	fctx := resp.GetContext()
	if fctx == nil || fctx.GetSessionId() == "" {
		return FlowSummary{}, fmt.Errorf("velociraptor: get flow status: %w", ErrFlowNotFound)
	}

	return toFlowSummaryFromContext(clientID, fctx), nil
}

// GetFlowResults first calls GetFlowDetails to discover the one
// artifact this flow collected (this project only ever starts
// single-artifact flows; see CollectArtifact), then calls Velociraptor's
// GetTable RPC for that artifact's result rows. All RPCs here are typed
// and parameterized only by already-validated client_id/flow_id values,
// a server-reported artifact name, and (v0.10.3+) a source name either
// supplied by the caller or discovered from the server's own artifact
// catalog — never a caller-supplied or constructed query.
//
// Named sources (v0.10.3): Velociraptor's GetTable RPC identifies a
// flow's result table by `Artifact`, which must be source-qualified
// ("ArtifactName/SourceName") for any artifact whose source has an
// explicit name — confirmed against upstream's
// paths.SplitFullSourceName and ArtifactPathManager.GetPathForWriting
// (paths/artifacts/paths.go), which split/join on exactly one "/" and
// write results under .../artifacts/<ArtifactName>/<FlowId>/<SourceName>
// when a source name is present, or .../artifacts/<ArtifactName>/<FlowId>
// when it is not. Before v0.10.3, this method always queried the bare
// artifact name, so any artifact compiling to a named source (e.g.
// Generic.Client.Info's BasicInformation/DetailedInfo/LinuxInfo — used
// by nearly every DFIR profile) silently returned zero rows even though
// the collection succeeded and real data existed server-side. See
// docs/live-validation-report-v0.10.2.md finding 2.
func (c *grpcClient) GetFlowResults(ctx context.Context, clientID, flowID, source string, maxRows int, maxBytes int64, cursor string) (FlowResultPage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	details, err := c.flows.GetFlowDetails(ctx, &veloapi.ApiFlowRequest{ClientId: clientID, FlowId: flowID})
	if err != nil {
		if isNotFoundError(err) {
			return FlowResultPage{}, fmt.Errorf("velociraptor: get flow results: %w", ErrFlowNotFound)
		}
		return FlowResultPage{}, fmt.Errorf("velociraptor: get flow results: %w", sanitizeTLSError(err))
	}
	fctx := details.GetContext()
	if fctx == nil || fctx.GetSessionId() == "" {
		return FlowResultPage{}, fmt.Errorf("velociraptor: get flow results: %w", ErrFlowNotFound)
	}

	artifact := firstArtifactName(fctx.GetRequest())
	if artifact == "" {
		// The flow exists but this project cannot tell which artifact it
		// collected (e.g. a flow not started by this project's own
		// CollectArtifact path); report no rows rather than guessing.
		return FlowResultPage{}, nil
	}

	// Source discovery is best-effort: a failure here (e.g. the artifact
	// was removed from the catalog since the flow ran) falls back to the
	// bare artifact name rather than failing the whole call.
	sources, _ := c.resolveArtifactSourceNames(ctx, artifact)
	tableArtifact, sourceRequired, availableSources, err := resolveResultArtifact(artifact, source, sources)
	if err != nil {
		return FlowResultPage{}, fmt.Errorf("velociraptor: get flow results: %w", err)
	}
	if sourceRequired {
		return FlowResultPage{SourceRequired: true, AvailableSources: availableSources}, nil
	}

	bounded := boundLimit(maxRows, c.effectiveMaxRows())
	startRow := parseOffsetCursor(cursor)

	resp, err := c.tables.GetTable(ctx, &veloapi.GetTableRequest{
		ClientId: clientID,
		FlowId:   flowID,
		Artifact: tableArtifact,
		Rows:     uint64(bounded),
		StartRow: uint64(startRow),
	})
	if err != nil {
		return FlowResultPage{}, fmt.Errorf("velociraptor: get flow results: %w", sanitizeTLSError(err))
	}

	rows, err := decodeTableRows(resp)
	if err != nil {
		return FlowResultPage{}, fmt.Errorf("velociraptor: get flow results: %w", err)
	}

	total := int(resp.GetTotalRows())
	truncated := len(rows) >= bounded && (total == 0 || startRow+len(rows) < total)
	page := FlowResultPage{
		Rows:         rows,
		TotalRows:    total,
		ReturnedRows: len(rows),
		Truncated:    truncated,
	}
	if truncated {
		page.NextCursor = offsetCursor(startRow + len(rows))
	}
	return page, nil
}

// CollectArtifact calls Velociraptor's CollectArtifact RPC to launch a
// single-artifact collection against one client.
func (c *grpcClient) CollectArtifact(ctx context.Context, req CollectionRequest) (FlowSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.flows.CollectArtifact(ctx, artifactCollectorArgsFor(req.ClientID, req.Artifact, req.Parameters))
	if err != nil {
		return FlowSummary{}, fmt.Errorf("velociraptor: collect artifact: %w", sanitizeTLSError(err))
	}

	return FlowSummary{
		FlowID:   resp.GetFlowId(),
		ClientID: req.ClientID,
		Artifact: req.Artifact,
		State:    FlowStateRunning,
	}, nil
}

// CancelFlow calls Velociraptor's CancelFlow RPC.
func (c *grpcClient) CancelFlow(ctx context.Context, clientID, flowID string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, err := c.flows.CancelFlow(ctx, &veloapi.ApiFlowRequest{ClientId: clientID, FlowId: flowID})
	if err != nil {
		return fmt.Errorf("velociraptor: cancel flow: %w", sanitizeTLSError(err))
	}
	return nil
}

// isNotFoundError is a best-effort heuristic for gRPC errors that
// represent "no such object" rather than a connectivity/permission
// failure, for RPCs (like GetHunt) that upstream Velociraptor answers
// with a real error status instead of a zero-value response. It is
// intentionally permissive (a plain substring match) since this
// project does not depend on Velociraptor's internal error-string
// format; a false negative here still surfaces as a normal error
// result, just not specifically NotFound.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
