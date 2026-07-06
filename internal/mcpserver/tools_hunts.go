package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/dfir"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/validation"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

// HuntTools manage multi-client hunts. Preview, listing, status, and
// results are read-only; starting/cancelling a hunt is approval-gated
// and must always be preceded by a scope preview so the approver sees
// blast radius (matched client count) before deciding.
var HuntTools = []ToolSpec{
	{
		Name:        "velo_preview_hunt_scope",
		Description: "Resolve a proposed hunt scope (client IDs, label, or all) against the current client population without starting anything.",
		ReadOnly:    true,
	},
	{
		Name:             "velo_start_hunt_with_approval",
		Description:      "Start a hunt for one allowlisted artifact across a previewed, bounded scope. Requires approval.",
		RequiresApproval: true,
	},
	{
		Name:             "velo_start_dfir_hunt_with_approval",
		Description:      "Start a hunt for every artifact in one allowlisted DFIR profile across a previewed, bounded scope. Requires approval.",
		RequiresApproval: true,
	},
	{
		Name:        "velo_list_hunts",
		Description: "List hunts.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_hunt_status",
		Description: "Get the state and client count of one hunt.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_hunt_results",
		Description: "Get result rows for one hunt, bounded by max_rows/max_result_bytes.",
		ReadOnly:    true,
	},
	{
		Name:             "velo_cancel_hunt_with_approval",
		Description:      "Stop a running hunt. Requires approval.",
		RequiresApproval: true,
	},
}

// HuntSummaryOutput mirrors velociraptor.HuntSummary with explicit JSON
// tags for the MCP tool response schema.
type HuntSummaryOutput struct {
	HuntID      string `json:"hunt_id"`
	Artifact    string `json:"artifact,omitempty"`
	State       string `json:"state,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	ClientCount int    `json:"client_count,omitempty"`
}

func toHuntSummaryOutput(h velociraptor.HuntSummary) HuntSummaryOutput {
	return HuntSummaryOutput{
		HuntID:      h.HuntID,
		Artifact:    h.Artifact,
		State:       string(h.State),
		CreatedAt:   h.CreatedAt,
		ClientCount: h.ClientCount,
	}
}

// HuntScopePreviewOutput mirrors velociraptor.HuntScopePreview.
type HuntScopePreviewOutput struct {
	response.Result
	Mode              string   `json:"mode"`
	Matched           int      `json:"matched"`
	SampleClientIDs   []string `json:"sample_client_ids,omitempty"`
	ExceedsMaxClients bool     `json:"exceeds_max_clients"`
	MaxClients        int      `json:"max_clients,omitempty"`
}

// ---------------------------------------------------------------------------
// velo_preview_hunt_scope
// ---------------------------------------------------------------------------

type PreviewHuntScopeInput struct {
	ClientIDs  []string `json:"client_ids,omitempty" jsonschema:"explicit client IDs to target"`
	Label      string   `json:"label,omitempty" jsonschema:"label filter, e.g. linux or windows"`
	All        bool     `json:"all,omitempty" jsonschema:"target all clients; blocked by default unless policy allows"`
	MaxClients int      `json:"max_clients,omitempty" jsonschema:"maximum clients to target; server-side ceiling applies"`
}

func newPreviewHuntScopeHandler(deps Deps) mcp.ToolHandlerFor[PreviewHuntScopeInput, HuntScopePreviewOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in PreviewHuntScopeInput) (*mcp.CallToolResult, HuntScopePreviewOutput, error) {
		if err := validateHuntScopeInput(in.ClientIDs, in.Label, in.All); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_preview_hunt_scope", Outcome: audit.OutcomeBlocked, Reason: err.Error()})
			return nil, HuntScopePreviewOutput{}, err
		}

		if in.All && (deps.Policy == nil || !deps.Policy.TargetAllAllowed()) {
			recordAudit(deps, audit.Event{Tool: "velo_preview_hunt_scope", Outcome: audit.OutcomeBlocked, Reason: "target_all is disabled by policy"})
			return nil, HuntScopePreviewOutput{}, fmt.Errorf("target_all is disabled by policy")
		}

		maxClients := configuredMaxHuntClients(deps)
		if in.MaxClients > 0 && in.MaxClients < maxClients {
			maxClients = in.MaxClients
		}

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{Tool: "velo_preview_hunt_scope", Outcome: audit.OutcomeSuccess, Reason: "mock mode, returning preview based on input only"})
			return nil, HuntScopePreviewOutput{
				Result:            response.Success("MCP server is running in mock mode; preview returns input-based estimate only"),
				Mode:              VelociraptorModeMock,
				Matched:           0,
				SampleClientIDs:   in.ClientIDs,
				ExceedsMaxClients: len(in.ClientIDs) > maxClients,
				MaxClients:        maxClients,
			}, nil
		}

		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{Tool: "velo_preview_hunt_scope", Outcome: audit.OutcomeError, Reason: "VelociraptorReadMode is real but ReadClient is nil"})
			return nil, HuntScopePreviewOutput{Result: response.Error("real mode is configured but no Velociraptor client is available"), Mode: VelociraptorModeReal}, nil
		}

		preview, err := deps.ReadClient.PreviewHuntScope(ctx, velociraptor.HuntScopeRequest{
			ClientIDs: in.ClientIDs,
			Label:     in.Label,
			All:       in.All,
		})
		if err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_preview_hunt_scope", Outcome: audit.OutcomeError, Reason: err.Error()})
			return nil, HuntScopePreviewOutput{Result: response.Error(err.Error()), Mode: VelociraptorModeReal}, nil
		}

		recordAudit(deps, audit.Event{Tool: "velo_preview_hunt_scope", Outcome: audit.OutcomeSuccess, RowCount: preview.MatchedClientCount})
		return nil, HuntScopePreviewOutput{
			Result:            response.Result{Status: response.StatusForCount(preview.MatchedClientCount)},
			Mode:              VelociraptorModeReal,
			Matched:           preview.MatchedClientCount,
			SampleClientIDs:   preview.SampleClientIDs,
			ExceedsMaxClients: preview.ExceedsMaxClients || preview.MatchedClientCount > maxClients,
			MaxClients:        maxClients,
		}, nil
	}
}

// ---------------------------------------------------------------------------
// velo_start_hunt_with_approval
// ---------------------------------------------------------------------------

type StartHuntInput struct {
	CaseID     string            `json:"case_id" jsonschema:"investigation case ID (required)"`
	Reason     string            `json:"reason" jsonschema:"justification for starting the hunt (required)"`
	Requester  string            `json:"requester" jsonschema:"person requesting the hunt (required)"`
	ApprovalID string            `json:"approval_id" jsonschema:"approval reference ID (required)"`
	Artifact   string            `json:"artifact" jsonschema:"artifact name to hunt, e.g. Windows.System.Pslist"`
	Parameters map[string]string `json:"parameters,omitempty" jsonschema:"artifact parameters"`
	ClientIDs  []string          `json:"client_ids,omitempty" jsonschema:"explicit client IDs to target"`
	Label      string            `json:"label,omitempty" jsonschema:"label filter"`
	All        bool              `json:"all,omitempty" jsonschema:"target all clients"`
	MaxClients int               `json:"max_clients,omitempty" jsonschema:"max clients cap"`
}

type StartHuntOutput struct {
	response.Result
	Mode      string `json:"mode"`
	HuntID    string `json:"hunt_id,omitempty"`
	Artifact  string `json:"artifact,omitempty"`
	State     string `json:"state,omitempty"`
	ScopeDesc string `json:"scope_desc,omitempty"`
}

func newStartHuntHandler(deps Deps) mcp.ToolHandlerFor[StartHuntInput, StartHuntOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in StartHuntInput) (*mcp.CallToolResult, StartHuntOutput, error) {
		if enabled, reason := writePilotEnabled(deps); !enabled {
			recordAudit(deps, audit.Event{Tool: "velo_start_hunt_with_approval", Outcome: audit.OutcomeBlocked, Artifact: in.Artifact, CaseID: in.CaseID, Reason: reason})
			return nil, StartHuntOutput{}, errors.New(reason)
		}
		if err := validateHuntWriteInput(deps, in.CaseID, in.Reason, in.ApprovalID); err != nil {
			return nil, StartHuntOutput{}, err
		}
		if err := validateHuntScopeInput(in.ClientIDs, in.Label, in.All); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_start_hunt_with_approval", Outcome: audit.OutcomeBlocked, Reason: err.Error()})
			return nil, StartHuntOutput{}, err
		}
		if err := validation.ArtifactName(in.Artifact); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_start_hunt_with_approval", Outcome: audit.OutcomeBlocked, Artifact: in.Artifact, Reason: err.Error()})
			return nil, StartHuntOutput{}, err
		}
		if deps.Policy != nil && !deps.Policy.ArtifactAllowed(in.Artifact) {
			recordAudit(deps, audit.Event{Tool: "velo_start_hunt_with_approval", Outcome: audit.OutcomeBlocked, Artifact: in.Artifact, Reason: "artifact not in allowlist"})
			return nil, StartHuntOutput{}, fmt.Errorf("artifact %q is not in the configured allowlist", in.Artifact)
		}

		if in.All && (deps.Policy == nil || !deps.Policy.TargetAllAllowed()) {
			recordAudit(deps, audit.Event{Tool: "velo_start_hunt_with_approval", Outcome: audit.OutcomeBlocked, Reason: "target_all is disabled by policy"})
			return nil, StartHuntOutput{}, fmt.Errorf("target_all is disabled by policy")
		}

		maxClients := configuredMaxHuntClients(deps)
		if in.MaxClients > 0 && in.MaxClients < maxClients {
			maxClients = in.MaxClients
		}

		candidate := approval.Request{
			Operation:  approval.OperationStartHunt,
			CaseID:     in.CaseID,
			Artifact:   in.Artifact,
			Parameters: in.Parameters,
			ClientIDs:  in.ClientIDs,
			Label:      in.Label,
			TargetAll:  in.All,
		}
		result, outcome, ok := verifyAndConsumeApproval(ctx, deps, in.ApprovalID, candidate)
		if !ok {
			recordAudit(deps, audit.Event{Tool: "velo_start_hunt_with_approval", Outcome: outcome, Artifact: in.Artifact, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalID, Reason: result.Message})
			return nil, StartHuntOutput{Result: result, Artifact: in.Artifact}, nil
		}

		if deps.WriteClient == nil {
			recordAudit(deps, audit.Event{Tool: "velo_start_hunt_with_approval", Outcome: audit.OutcomeError, Artifact: in.Artifact, CaseID: in.CaseID, ApprovalID: in.ApprovalID, Reason: "no write client configured"})
			return nil, StartHuntOutput{Result: response.Error("real mode is configured but no Velociraptor write client is available"), Artifact: in.Artifact}, nil
		}

		hunt, err := deps.WriteClient.StartHunt(ctx, velociraptor.HuntRequest{
			Artifact:   in.Artifact,
			Parameters: in.Parameters,
			Scope: velociraptor.HuntScopeRequest{
				ClientIDs: in.ClientIDs,
				Label:     in.Label,
				All:       in.All,
			},
			MaxClients: maxClients,
		})
		if err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_start_hunt_with_approval", Outcome: audit.OutcomeError, Artifact: in.Artifact, Reason: err.Error()})
			return nil, StartHuntOutput{Result: response.Error(err.Error()), Mode: VelociraptorModeReal, Artifact: in.Artifact}, nil
		}

		scopeDesc := describeScope(in.ClientIDs, in.Label, in.All)
		recordAudit(deps, audit.Event{Tool: "velo_start_hunt_with_approval", Outcome: audit.OutcomeSuccess, HuntID: hunt.HuntID, Artifact: in.Artifact, CaseID: in.CaseID, ApprovalID: in.ApprovalID})
		return nil, StartHuntOutput{
			Result:    response.Success("hunt started"),
			Mode:      VelociraptorModeReal,
			HuntID:    hunt.HuntID,
			Artifact:  in.Artifact,
			State:     string(hunt.State),
			ScopeDesc: scopeDesc,
		}, nil
	}
}

// ---------------------------------------------------------------------------
// velo_start_dfir_hunt_with_approval
// ---------------------------------------------------------------------------

type StartDFIRHuntInput struct {
	CaseID     string   `json:"case_id" jsonschema:"investigation case ID (required)"`
	Reason     string   `json:"reason" jsonschema:"justification for starting the hunt (required)"`
	Requester  string   `json:"requester" jsonschema:"person requesting the hunt (required)"`
	ApprovalID string   `json:"approval_id" jsonschema:"approval reference ID (required)"`
	Profile    string   `json:"profile" jsonschema:"DFIR profile name, e.g. windows_basic_triage"`
	ClientIDs  []string `json:"client_ids,omitempty" jsonschema:"explicit client IDs to target"`
	Label      string   `json:"label,omitempty" jsonschema:"label filter"`
	All        bool     `json:"all,omitempty" jsonschema:"target all clients"`
	MaxClients int      `json:"max_clients,omitempty" jsonschema:"max clients cap"`
}

type StartDFIRHuntOutput struct {
	response.Result
	Mode      string `json:"mode"`
	HuntID    string `json:"hunt_id,omitempty"`
	Profile   string `json:"profile,omitempty"`
	State     string `json:"state,omitempty"`
	ScopeDesc string `json:"scope_desc,omitempty"`
}

func newStartDFIRHuntHandler(deps Deps) mcp.ToolHandlerFor[StartDFIRHuntInput, StartDFIRHuntOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in StartDFIRHuntInput) (*mcp.CallToolResult, StartDFIRHuntOutput, error) {
		if enabled, reason := writePilotEnabled(deps); !enabled {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeBlocked, Profile: in.Profile, CaseID: in.CaseID, Reason: reason})
			return nil, StartDFIRHuntOutput{}, errors.New(reason)
		}
		if err := validateHuntWriteInput(deps, in.CaseID, in.Reason, in.ApprovalID); err != nil {
			return nil, StartDFIRHuntOutput{}, err
		}
		if err := validateHuntScopeInput(in.ClientIDs, in.Label, in.All); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeBlocked, Reason: err.Error()})
			return nil, StartDFIRHuntOutput{}, err
		}
		if err := validation.DFIRProfileName(in.Profile); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeBlocked, Profile: in.Profile, Reason: err.Error()})
			return nil, StartDFIRHuntOutput{}, err
		}
		if deps.Policy != nil && !deps.Policy.ProfileAllowed(in.Profile) {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeBlocked, Profile: in.Profile, Reason: "profile not in allowlist"})
			return nil, StartDFIRHuntOutput{}, fmt.Errorf("profile %q is not in the configured allowlist", in.Profile)
		}
		if deps.Profiles == nil {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeBlocked, Profile: in.Profile, Reason: "profile registry not configured"})
			return nil, StartDFIRHuntOutput{}, fmt.Errorf("profile registry is not configured")
		}

		prof, ok := deps.Profiles.Get(in.Profile)
		if !ok {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeError, Profile: in.Profile, Reason: "profile not found"})
			return nil, StartDFIRHuntOutput{Result: response.NotFound(fmt.Sprintf("profile %q not found", in.Profile)), Mode: VelociraptorModeReal, Profile: in.Profile}, nil
		}

		if err := dfir.ValidateProfile(prof, deps.Policy); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeBlocked, Profile: in.Profile, Reason: err.Error()})
			return nil, StartDFIRHuntOutput{}, err
		}

		if in.All && (deps.Policy == nil || !deps.Policy.TargetAllAllowed()) {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeBlocked, Reason: "target_all is disabled by policy"})
			return nil, StartDFIRHuntOutput{}, fmt.Errorf("target_all is disabled by policy")
		}

		maxClients := configuredMaxHuntClients(deps)
		if in.MaxClients > 0 && in.MaxClients < maxClients {
			maxClients = in.MaxClients
		}

		candidate := approval.Request{
			Operation: approval.OperationStartDFIRHunt,
			CaseID:    in.CaseID,
			Profile:   in.Profile,
			ClientIDs: in.ClientIDs,
			Label:     in.Label,
			TargetAll: in.All,
		}
		result, outcome, ok := verifyAndConsumeApproval(ctx, deps, in.ApprovalID, candidate)
		if !ok {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: outcome, Profile: in.Profile, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalID, Reason: result.Message})
			return nil, StartDFIRHuntOutput{Result: result, Profile: in.Profile}, nil
		}

		if deps.WriteClient == nil {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeError, Profile: in.Profile, CaseID: in.CaseID, ApprovalID: in.ApprovalID, Reason: "no write client configured"})
			return nil, StartDFIRHuntOutput{Result: response.Error("real mode is configured but no Velociraptor write client is available"), Profile: in.Profile}, nil
		}

		// Collect all artifacts from the profile
		var lastHunt velociraptor.HuntSummary
		var lastErr error
		for _, art := range prof.Artifacts {
			params := make(map[string]string)
			for k, v := range art.Parameters {
				params[k] = v
			}
			hunt, err := deps.WriteClient.StartHunt(ctx, velociraptor.HuntRequest{
				Artifact:   art.Name,
				Parameters: params,
				Scope: velociraptor.HuntScopeRequest{
					ClientIDs: in.ClientIDs,
					Label:     in.Label,
					All:       in.All,
				},
				MaxClients: maxClients,
			})
			if err != nil {
				lastErr = err
				break
			}
			lastHunt = hunt
		}
		if lastErr != nil {
			recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeError, Profile: in.Profile, Reason: lastErr.Error()})
			return nil, StartDFIRHuntOutput{Result: response.Error(lastErr.Error()), Mode: VelociraptorModeReal, Profile: in.Profile}, nil
		}

		scopeDesc := describeScope(in.ClientIDs, in.Label, in.All)
		recordAudit(deps, audit.Event{Tool: "velo_start_dfir_hunt_with_approval", Outcome: audit.OutcomeSuccess, HuntID: lastHunt.HuntID, Profile: in.Profile, CaseID: in.CaseID, ApprovalID: in.ApprovalID})
		return nil, StartDFIRHuntOutput{
			Result:    response.Success("DFIR profile hunt started"),
			Mode:      VelociraptorModeReal,
			HuntID:    lastHunt.HuntID,
			Profile:   in.Profile,
			State:     string(lastHunt.State),
			ScopeDesc: scopeDesc,
		}, nil
	}
}

// ---------------------------------------------------------------------------
// velo_list_hunts
// ---------------------------------------------------------------------------

type ListHuntsInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum number of hunts to return; server-side ceiling applies"`
	Cursor string `json:"cursor,omitempty" jsonschema:"opaque pagination cursor returned by a previous call"`
}

type ListHuntsOutput struct {
	response.Result
	Mode       string              `json:"mode"`
	Hunts      []HuntSummaryOutput `json:"hunts"`
	NextCursor string              `json:"next_cursor,omitempty"`
	Truncated  bool                `json:"truncated"`
}

func newListHuntsHandler(deps Deps) mcp.ToolHandlerFor[ListHuntsInput, ListHuntsOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ListHuntsInput) (*mcp.CallToolResult, ListHuntsOutput, error) {
		limit := boundToolLimit(in.Limit, configuredMaxRows(deps))

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{Tool: "velo_list_hunts", Outcome: audit.OutcomeSuccess, Reason: "mock mode, no Velociraptor call made"})
			return nil, ListHuntsOutput{Result: response.Success("MCP server is running in mock mode; no Velociraptor call was made"), Mode: VelociraptorModeMock, Hunts: []HuntSummaryOutput{}}, nil
		}

		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{Tool: "velo_list_hunts", Outcome: audit.OutcomeError, Reason: "VelociraptorReadMode is real but ReadClient is nil"})
			return nil, ListHuntsOutput{Result: response.Error("real mode is configured but no Velociraptor client is available"), Mode: VelociraptorModeReal, Hunts: []HuntSummaryOutput{}}, nil
		}

		hunts, err := deps.ReadClient.ListHunts(ctx, limit)
		if err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_list_hunts", Outcome: audit.OutcomeError, Reason: err.Error()})
			return nil, ListHuntsOutput{Result: response.Error(err.Error()), Mode: VelociraptorModeReal, Hunts: []HuntSummaryOutput{}}, nil
		}

		truncated := len(hunts) > limit
		if truncated {
			hunts = hunts[:limit]
		}
		out := make([]HuntSummaryOutput, 0, len(hunts))
		for _, h := range hunts {
			out = append(out, toHuntSummaryOutput(h))
		}

		recordAudit(deps, audit.Event{Tool: "velo_list_hunts", Outcome: audit.OutcomeSuccess, RowCount: len(out)})
		return nil, ListHuntsOutput{
			Result:     response.Result{Status: response.StatusForCount(len(out))},
			Mode:       VelociraptorModeReal,
			Hunts:      out,
			NextCursor: nextOffsetCursor(in.Cursor, len(out), truncated),
			Truncated:  truncated,
		}, nil
	}
}

// ---------------------------------------------------------------------------
// velo_get_hunt_status
// ---------------------------------------------------------------------------

type GetHuntStatusInput struct {
	HuntID string `json:"hunt_id" jsonschema:"Velociraptor hunt ID, e.g. H.1234abcd5678ef90"`
}

type GetHuntStatusOutput struct {
	response.Result
	Mode string             `json:"mode"`
	Hunt *HuntSummaryOutput `json:"hunt,omitempty"`
}

func newGetHuntStatusHandler(deps Deps) mcp.ToolHandlerFor[GetHuntStatusInput, GetHuntStatusOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in GetHuntStatusInput) (*mcp.CallToolResult, GetHuntStatusOutput, error) {
		if in.HuntID == "" {
			recordAudit(deps, audit.Event{Tool: "velo_get_hunt_status", Outcome: audit.OutcomeBlocked, Reason: "missing hunt_id"})
			return nil, GetHuntStatusOutput{}, fmt.Errorf("hunt_id is required")
		}

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{Tool: "velo_get_hunt_status", Outcome: audit.OutcomeSuccess, HuntID: in.HuntID, Reason: "mock mode, no Velociraptor call made"})
			return nil, GetHuntStatusOutput{Result: response.Success("MCP server is running in mock mode; no Velociraptor call was made"), Mode: VelociraptorModeMock}, nil
		}

		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{Tool: "velo_get_hunt_status", Outcome: audit.OutcomeError, HuntID: in.HuntID, Reason: "VelociraptorReadMode is real but ReadClient is nil"})
			return nil, GetHuntStatusOutput{Result: response.Error("real mode is configured but no Velociraptor client is available"), Mode: VelociraptorModeReal}, nil
		}

		hunt, err := deps.ReadClient.GetHuntStatus(ctx, in.HuntID)
		if err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_get_hunt_status", Outcome: audit.OutcomeError, HuntID: in.HuntID, Reason: err.Error()})
			result := response.Error(err.Error())
			if errors.Is(err, velociraptor.ErrHuntNotFound) {
				result = response.NotFound(err.Error())
			}
			return nil, GetHuntStatusOutput{Result: result, Mode: VelociraptorModeReal}, nil
		}

		out := toHuntSummaryOutput(hunt)
		recordAudit(deps, audit.Event{Tool: "velo_get_hunt_status", Outcome: audit.OutcomeSuccess, HuntID: in.HuntID})
		return nil, GetHuntStatusOutput{Result: response.Result{Status: response.StatusSuccess}, Mode: VelociraptorModeReal, Hunt: &out}, nil
	}
}

// ---------------------------------------------------------------------------
// velo_get_hunt_results
// ---------------------------------------------------------------------------

type GetHuntResultsInput struct {
	HuntID string `json:"hunt_id" jsonschema:"Velociraptor hunt ID, e.g. H.1234abcd5678ef90"`
	Limit  int    `json:"limit,omitempty" jsonschema:"maximum result rows to return; server-side max_rows ceiling applies"`
	Cursor string `json:"cursor,omitempty" jsonschema:"opaque pagination cursor returned by a previous call"`
}

type GetHuntResultsOutput struct {
	response.Result
	Mode         string           `json:"mode"`
	HuntID       string           `json:"hunt_id"`
	Rows         []map[string]any `json:"rows"`
	ReturnedRows int              `json:"returned_rows"`
	TotalRows    int              `json:"total_rows,omitempty"`
	ByteCount    int64            `json:"byte_count"`
	NextCursor   string           `json:"next_cursor,omitempty"`
	Truncated    bool             `json:"truncated"`
}

func newGetHuntResultsHandler(deps Deps) mcp.ToolHandlerFor[GetHuntResultsInput, GetHuntResultsOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in GetHuntResultsInput) (*mcp.CallToolResult, GetHuntResultsOutput, error) {
		if in.HuntID == "" {
			recordAudit(deps, audit.Event{Tool: "velo_get_hunt_results", Outcome: audit.OutcomeBlocked, Reason: "missing hunt_id"})
			return nil, GetHuntResultsOutput{}, fmt.Errorf("hunt_id is required")
		}

		limit := boundToolLimit(in.Limit, configuredMaxRows(deps))
		maxBytes := configuredMaxResultBytes(deps)

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{Tool: "velo_get_hunt_results", Outcome: audit.OutcomeSuccess, HuntID: in.HuntID, Reason: "mock mode, no Velociraptor call made"})
			return nil, GetHuntResultsOutput{Result: response.Success("MCP server is running in mock mode; no Velociraptor call was made"), Mode: VelociraptorModeMock, HuntID: in.HuntID, Rows: []map[string]any{}}, nil
		}

		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{Tool: "velo_get_hunt_results", Outcome: audit.OutcomeError, HuntID: in.HuntID, Reason: "VelociraptorReadMode is real but ReadClient is nil"})
			return nil, GetHuntResultsOutput{Result: response.Error("real mode is configured but no Velociraptor client is available"), Mode: VelociraptorModeReal, HuntID: in.HuntID, Rows: []map[string]any{}}, nil
		}

		page, err := deps.ReadClient.GetHuntResults(ctx, in.HuntID, limit, maxBytes)
		if err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_get_hunt_results", Outcome: audit.OutcomeError, HuntID: in.HuntID, Reason: err.Error()})
			result := response.Error(err.Error())
			if errors.Is(err, velociraptor.ErrHuntNotFound) {
				result = response.NotFound(err.Error())
			}
			return nil, GetHuntResultsOutput{Result: result, Mode: VelociraptorModeReal, HuntID: in.HuntID, Rows: []map[string]any{}}, nil
		}

		rows, byteCount, truncatedByHandler := boundRowsByLimitAndBytes(page.Rows, limit, maxBytes)
		truncated := page.Truncated || truncatedByHandler
		totalRows := page.TotalRows
		if totalRows == 0 && len(page.Rows) > 0 {
			totalRows = len(page.Rows)
		}

		result := response.Result{Status: response.StatusForCount(len(rows))}
		if len(rows) == 0 && truncatedByHandler && len(page.Rows) > 0 {
			result = response.Error("hunt result rows exceed configured max_result_bytes")
		}

		recordAudit(deps, audit.Event{Tool: "velo_get_hunt_results", Outcome: audit.OutcomeSuccess, HuntID: in.HuntID, RowCount: len(rows), ByteCount: byteCount})
		return nil, GetHuntResultsOutput{
			Result:       result,
			Mode:         VelociraptorModeReal,
			HuntID:       in.HuntID,
			Rows:         rows,
			ReturnedRows: len(rows),
			TotalRows:    totalRows,
			ByteCount:    byteCount,
			NextCursor:   nextOffsetCursor(in.Cursor, len(rows), truncated),
			Truncated:    truncated,
		}, nil
	}
}

// ---------------------------------------------------------------------------
// velo_cancel_hunt_with_approval
// ---------------------------------------------------------------------------

type CancelHuntInput struct {
	CaseID     string `json:"case_id" jsonschema:"investigation case ID (required)"`
	Reason     string `json:"reason" jsonschema:"justification for cancelling the hunt (required)"`
	Requester  string `json:"requester" jsonschema:"person requesting cancellation (required)"`
	ApprovalID string `json:"approval_id" jsonschema:"approval reference ID (required)"`
	HuntID     string `json:"hunt_id" jsonschema:"hunt ID to cancel, e.g. H.1234abcd5678ef90"`
}

type CancelHuntOutput struct {
	response.Result
	Mode   string `json:"mode"`
	HuntID string `json:"hunt_id"`
}

func newCancelHuntHandler(deps Deps) mcp.ToolHandlerFor[CancelHuntInput, CancelHuntOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in CancelHuntInput) (*mcp.CallToolResult, CancelHuntOutput, error) {
		if enabled, reason := writePilotEnabled(deps); !enabled {
			recordAudit(deps, audit.Event{Tool: "velo_cancel_hunt_with_approval", Outcome: audit.OutcomeBlocked, HuntID: in.HuntID, CaseID: in.CaseID, Reason: reason})
			return nil, CancelHuntOutput{}, errors.New(reason)
		}
		if err := validateHuntWriteInput(deps, in.CaseID, in.Reason, in.ApprovalID); err != nil {
			return nil, CancelHuntOutput{}, err
		}
		if in.HuntID == "" {
			recordAudit(deps, audit.Event{Tool: "velo_cancel_hunt_with_approval", Outcome: audit.OutcomeBlocked, Reason: "missing hunt_id"})
			return nil, CancelHuntOutput{}, fmt.Errorf("hunt_id is required")
		}

		candidate := approval.Request{
			Operation: approval.OperationCancelHunt,
			CaseID:    in.CaseID,
			HuntID:    in.HuntID,
		}
		result, outcome, ok := verifyAndConsumeApproval(ctx, deps, in.ApprovalID, candidate)
		if !ok {
			recordAudit(deps, audit.Event{Tool: "velo_cancel_hunt_with_approval", Outcome: outcome, HuntID: in.HuntID, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalID, Reason: result.Message})
			return nil, CancelHuntOutput{Result: result, HuntID: in.HuntID}, nil
		}

		if deps.WriteClient == nil {
			recordAudit(deps, audit.Event{Tool: "velo_cancel_hunt_with_approval", Outcome: audit.OutcomeError, HuntID: in.HuntID, CaseID: in.CaseID, ApprovalID: in.ApprovalID, Reason: "no write client configured"})
			return nil, CancelHuntOutput{Result: response.Error("real mode is configured but no Velociraptor write client is available"), HuntID: in.HuntID}, nil
		}

		if err := deps.WriteClient.CancelHunt(ctx, in.HuntID); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_cancel_hunt_with_approval", Outcome: audit.OutcomeError, HuntID: in.HuntID, Reason: err.Error()})
			return nil, CancelHuntOutput{Result: response.Error(err.Error()), Mode: VelociraptorModeReal, HuntID: in.HuntID}, nil
		}

		recordAudit(deps, audit.Event{Tool: "velo_cancel_hunt_with_approval", Outcome: audit.OutcomeSuccess, HuntID: in.HuntID, CaseID: in.CaseID, ApprovalID: in.ApprovalID})
		return nil, CancelHuntOutput{Result: response.Success("hunt cancelled"), Mode: VelociraptorModeReal, HuntID: in.HuntID}, nil
	}
}

// ---------------------------------------------------------------------------
// Registration
// ---------------------------------------------------------------------------

func registerHuntTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_preview_hunt_scope",
		Description: HuntTools[0].Description,
		Annotations: readOnlyAnnotations("Preview hunt scope"),
	}, newPreviewHuntScopeHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_start_hunt_with_approval",
		Description: HuntTools[1].Description,
		Annotations: writeAnnotations("Start hunt (approval-gated)"),
	}, newStartHuntHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_start_dfir_hunt_with_approval",
		Description: HuntTools[2].Description,
		Annotations: writeAnnotations("Start DFIR profile hunt (approval-gated)"),
	}, newStartDFIRHuntHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_list_hunts",
		Description: HuntTools[3].Description,
		Annotations: readOnlyAnnotations("List hunts"),
	}, newListHuntsHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_get_hunt_status",
		Description: HuntTools[4].Description,
		Annotations: readOnlyAnnotations("Get hunt status"),
	}, newGetHuntStatusHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_get_hunt_results",
		Description: HuntTools[5].Description,
		Annotations: readOnlyAnnotations("Get hunt results"),
	}, newGetHuntResultsHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_cancel_hunt_with_approval",
		Description: HuntTools[6].Description,
		Annotations: writeAnnotations("Cancel hunt (approval-gated)"),
	}, newCancelHuntHandler(deps))
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func validateHuntScopeInput(clientIDs []string, label string, all bool) error {
	return validation.ValidateHuntScope(validation.HuntScope{
		ClientIDs: clientIDs,
		Label:     label,
		All:       all,
	})
}

func validateHuntWriteInput(deps Deps, caseID, reason, approvalID string) error {
	if caseID == "" {
		return fmt.Errorf("case_id is required")
	}
	if reason == "" {
		return fmt.Errorf("reason is required")
	}
	if approvalID == "" {
		return fmt.Errorf("approval_id is required")
	}
	if deps.Approvals == nil {
		return fmt.Errorf("approval store not configured; approval workflow is unavailable")
	}
	return nil
}

func configuredMaxHuntClients(deps Deps) int {
	if deps.Policy != nil {
		m := deps.Policy.MaxHuntClients()
		if m > 0 {
			return m
		}
	}
	return 100
}

func describeScope(clientIDs []string, label string, all bool) string {
	switch {
	case len(clientIDs) > 0:
		return fmt.Sprintf("%d explicit client(s)", len(clientIDs))
	case label != "":
		return fmt.Sprintf("label %q", label)
	case all:
		return "all clients"
	default:
		return "unknown"
	}
}
