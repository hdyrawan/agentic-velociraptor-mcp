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

// CollectionTools cover starting or stopping artifact collection on a
// single client. Every member here mutates endpoint-facing state and
// therefore requires case_id, reason, requester, and an
// approval_reference that must resolve to a matching, approved,
// unconsumed, unexpired approval.Store record before any Velociraptor
// call is made; see writePilotEnabled and verifyAndConsumeApproval in
// server.go.
var CollectionTools = []ToolSpec{
	{
		Name:             "velo_collect_artifact_with_approval",
		Description:      "Collect one allowlisted artifact from one client. Requires case_id, reason, requester, and a pre-approved approval_reference.",
		RequiresApproval: true,
	},
	{
		Name:             "velo_collect_dfir_profile_with_approval",
		Description:      "Collect every artifact in one allowlisted DFIR profile from one client. Requires case_id, reason, requester, and a pre-approved approval_reference.",
		RequiresApproval: true,
	},
	{
		Name:             "velo_cancel_flow_with_approval",
		Description:      "Cancel a running flow on a client. Requires case_id, reason, requester, and a pre-approved approval_reference.",
		RequiresApproval: true,
	},
}

func registerCollectionTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_collect_artifact_with_approval",
		Description: CollectionTools[0].Description,
		Annotations: writeAnnotations("Collect Velociraptor artifact (approval-gated)"),
	}, newCollectArtifactHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_collect_dfir_profile_with_approval",
		Description: CollectionTools[1].Description,
		Annotations: writeAnnotations("Collect DFIR profile (approval-gated)"),
	}, newCollectDFIRProfileHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_cancel_flow_with_approval",
		Description: CollectionTools[2].Description,
		Annotations: writeAnnotations("Cancel Velociraptor flow (approval-gated)"),
	}, newCancelFlowHandler(deps))
}

// validateApprovalFields validates the fields required on every
// approval-gated tool call: client_id, case_id, reason, requester, and
// the approval_reference itself.
func validateApprovalFields(clientID, caseID, reason, requester, reference string) error {
	if err := validation.ClientID(clientID); err != nil {
		return fmt.Errorf("invalid client id %q", clientID)
	}
	if err := validation.CaseID(caseID); err != nil {
		return err
	}
	if err := validation.Reason(reason); err != nil {
		return err
	}
	if err := validation.Requester(requester); err != nil {
		return err
	}
	if err := validation.ApprovalReference(reference); err != nil {
		return err
	}
	return nil
}

// CollectArtifactInput is velo_collect_artifact_with_approval's argument
// shape. ApprovalReference must name an approval.Store record created
// and approved out-of-band (via the agentic-velociraptor-mcp `approve`
// CLI subcommand) whose operation, case_id, client_id, artifact, and
// parameters exactly match this call's; see approval.RequestFingerprint.
type CollectArtifactInput struct {
	ClientID          string            `json:"client_id" jsonschema:"Velociraptor client ID, e.g. C.1234abcd5678ef90"`
	Artifact          string            `json:"artifact" jsonschema:"allowlisted Velociraptor artifact name, e.g. Windows.System.Pslist"`
	Parameters        map[string]string `json:"parameters,omitempty" jsonschema:"optional artifact parameters, bound as safe values, never raw VQL"`
	CaseID            string            `json:"case_id" jsonschema:"investigation/case identifier this collection is tied to"`
	Reason            string            `json:"reason" jsonschema:"justification for why this collection is needed"`
	Requester         string            `json:"requester" jsonschema:"identity of whoever is asking for this collection"`
	ApprovalReference string            `json:"approval_reference" jsonschema:"reference to a pre-approved request created via the approve CLI subcommand"`
}

// CollectArtifactOutput reports the outcome of an artifact collection
// attempt via the v0.2.0 response.Result envelope.
type CollectArtifactOutput struct {
	response.Result
	ClientID  string `json:"client_id,omitempty"`
	Artifact  string `json:"artifact,omitempty"`
	FlowID    string `json:"flow_id,omitempty"`
	State     string `json:"state,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

func newCollectArtifactHandler(deps Deps) mcp.ToolHandlerFor[CollectArtifactInput, CollectArtifactOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in CollectArtifactInput) (*mcp.CallToolResult, CollectArtifactOutput, error) {
		const tool = "velo_collect_artifact_with_approval"

		if enabled, reason := writePilotEnabled(deps); !enabled {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Artifact: in.Artifact, CaseID: in.CaseID, Reason: reason})
			return nil, CollectArtifactOutput{}, errors.New(reason)
		}

		if err := validateApprovalFields(in.ClientID, in.CaseID, in.Reason, in.Requester, in.ApprovalReference); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Artifact: in.Artifact, CaseID: in.CaseID, Reason: err.Error()})
			return nil, CollectArtifactOutput{}, err
		}
		if err := validation.ArtifactName(in.Artifact); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Artifact: in.Artifact, CaseID: in.CaseID, Reason: "invalid artifact name syntax"})
			return nil, CollectArtifactOutput{}, fmt.Errorf("invalid artifact name %q", in.Artifact)
		}
		if err := validation.CollectionParameters(in.Parameters); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Artifact: in.Artifact, CaseID: in.CaseID, Reason: err.Error()})
			return nil, CollectArtifactOutput{}, err
		}
		if deps.Policy == nil || !deps.Policy.ArtifactAllowed(in.Artifact) {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Artifact: in.Artifact, CaseID: in.CaseID, Reason: "artifact not in allowlist"})
			return nil, CollectArtifactOutput{}, fmt.Errorf("artifact %q is not in the configured allowlist", in.Artifact)
		}

		candidate := approval.Request{
			Operation:  approval.OperationCollectArtifact,
			CaseID:     in.CaseID,
			ClientID:   in.ClientID,
			Artifact:   in.Artifact,
			Parameters: in.Parameters,
		}
		result, outcome, ok := verifyAndConsumeApproval(ctx, deps, in.ApprovalReference, candidate)
		if !ok {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: outcome, ClientID: in.ClientID, Artifact: in.Artifact, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference, Reason: result.Message})
			return nil, CollectArtifactOutput{Result: result, ClientID: in.ClientID, Artifact: in.Artifact}, nil
		}

		if deps.WriteClient == nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, Artifact: in.Artifact, CaseID: in.CaseID, ApprovalID: in.ApprovalReference, Reason: "no Velociraptor write client is configured"})
			return nil, CollectArtifactOutput{
				Result:   response.Error("real mode is configured but no Velociraptor write client is available"),
				ClientID: in.ClientID,
				Artifact: in.Artifact,
			}, nil
		}

		summary, err := deps.WriteClient.CollectArtifact(ctx, velociraptor.CollectionRequest{
			ClientID:   in.ClientID,
			Artifact:   in.Artifact,
			Parameters: in.Parameters,
		})
		if err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, Artifact: in.Artifact, CaseID: in.CaseID, ApprovalID: in.ApprovalReference, Reason: err.Error()})
			return nil, CollectArtifactOutput{
				Result:   response.Error(err.Error()),
				ClientID: in.ClientID,
				Artifact: in.Artifact,
			}, nil
		}

		recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, Artifact: in.Artifact, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference, FlowID: summary.FlowID})
		return nil, CollectArtifactOutput{
			Result:    response.Result{Status: response.StatusSuccess},
			ClientID:  in.ClientID,
			Artifact:  in.Artifact,
			FlowID:    summary.FlowID,
			State:     string(summary.State),
			CreatedAt: summary.CreatedAt,
		}, nil
	}
}

// CollectDFIRProfileInput is velo_collect_dfir_profile_with_approval's
// argument shape. Unlike single-artifact collection, agent-supplied
// per-call Parameters are not accepted: a profile's artifacts carry
// their own reviewed, fixed parameters (dfir.ProfileArtifact.Parameters).
type CollectDFIRProfileInput struct {
	ClientID          string `json:"client_id" jsonschema:"Velociraptor client ID, e.g. C.1234abcd5678ef90"`
	Profile           string `json:"profile" jsonschema:"allowlisted DFIR profile name, e.g. windows_basic_triage"`
	CaseID            string `json:"case_id" jsonschema:"investigation/case identifier this collection is tied to"`
	Reason            string `json:"reason" jsonschema:"justification for why this collection is needed"`
	Requester         string `json:"requester" jsonschema:"identity of whoever is asking for this collection"`
	ApprovalReference string `json:"approval_reference" jsonschema:"reference to a pre-approved request created via the approve CLI subcommand"`
}

// CollectedFlow reports one artifact's collection attempt within a
// profile collection, so a partial failure partway through a profile is
// reported honestly rather than silently rolled up into a single
// success/failure bit.
type CollectedFlow struct {
	Artifact string `json:"artifact"`
	FlowID   string `json:"flow_id,omitempty"`
	State    string `json:"state,omitempty"`
	Error    string `json:"error,omitempty"`
}

// CollectDFIRProfileOutput reports the outcome of a DFIR profile
// collection attempt via the v0.2.0 response.Result envelope. Flows is
// populated even on a partial failure (Result.Status will be
// response.StatusError), so a caller can see exactly which artifacts
// succeeded before the failure occurred.
type CollectDFIRProfileOutput struct {
	response.Result
	ClientID string          `json:"client_id,omitempty"`
	Profile  string          `json:"profile,omitempty"`
	Flows    []CollectedFlow `json:"flows"`
}

func newCollectDFIRProfileHandler(deps Deps) mcp.ToolHandlerFor[CollectDFIRProfileInput, CollectDFIRProfileOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in CollectDFIRProfileInput) (*mcp.CallToolResult, CollectDFIRProfileOutput, error) {
		const tool = "velo_collect_dfir_profile_with_approval"

		if enabled, reason := writePilotEnabled(deps); !enabled {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, Reason: reason})
			return nil, CollectDFIRProfileOutput{}, errors.New(reason)
		}

		if err := validation.ClientID(in.ClientID); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, Reason: "invalid client id syntax"})
			return nil, CollectDFIRProfileOutput{}, fmt.Errorf("invalid client id %q", in.ClientID)
		}
		if err := validation.CaseID(in.CaseID); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Profile: in.Profile, Reason: err.Error()})
			return nil, CollectDFIRProfileOutput{}, err
		}
		if err := validation.Reason(in.Reason); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, Reason: err.Error()})
			return nil, CollectDFIRProfileOutput{}, err
		}
		if err := validation.Requester(in.Requester); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, Reason: err.Error()})
			return nil, CollectDFIRProfileOutput{}, err
		}
		if err := validation.ApprovalReference(in.ApprovalReference); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, Reason: err.Error()})
			return nil, CollectDFIRProfileOutput{}, err
		}
		if err := validation.DFIRProfileName(in.Profile); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, Reason: "invalid profile name syntax"})
			return nil, CollectDFIRProfileOutput{}, fmt.Errorf("invalid dfir profile name %q", in.Profile)
		}

		if deps.Policy == nil || !deps.Policy.ProfileAllowed(in.Profile) {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, Reason: "profile not in allowlist"})
			return nil, CollectDFIRProfileOutput{}, fmt.Errorf("dfir profile %q is not in the configured allowlist", in.Profile)
		}
		if deps.Profiles == nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, Reason: "dfir profile registry not configured"})
			return nil, CollectDFIRProfileOutput{}, fmt.Errorf("dfir profile registry is not configured")
		}
		profile, ok := deps.Profiles.Get(in.Profile)
		if !ok {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, Reason: "profile not found"})
			return nil, CollectDFIRProfileOutput{}, fmt.Errorf("dfir profile %q not found", in.Profile)
		}
		if err := dfir.ValidateProfile(profile, deps.Policy); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, Reason: err.Error()})
			return nil, CollectDFIRProfileOutput{}, fmt.Errorf("dfir profile %q fails artifact allowlist validation: %w", in.Profile, err)
		}

		candidate := approval.Request{
			Operation: approval.OperationCollectDFIRProfile,
			CaseID:    in.CaseID,
			ClientID:  in.ClientID,
			Profile:   in.Profile,
		}
		result, outcome, ok := verifyAndConsumeApproval(ctx, deps, in.ApprovalReference, candidate)
		if !ok {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: outcome, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference, Reason: result.Message})
			return nil, CollectDFIRProfileOutput{Result: result, ClientID: in.ClientID, Profile: in.Profile, Flows: []CollectedFlow{}}, nil
		}

		if deps.WriteClient == nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, ApprovalID: in.ApprovalReference, Reason: "no Velociraptor write client is configured"})
			return nil, CollectDFIRProfileOutput{
				Result:   response.Error("real mode is configured but no Velociraptor write client is available"),
				ClientID: in.ClientID,
				Profile:  in.Profile,
				Flows:    []CollectedFlow{},
			}, nil
		}

		flows := make([]CollectedFlow, 0, len(profile.Artifacts))
		var firstErr error
		for _, artifact := range profile.Artifacts {
			summary, err := deps.WriteClient.CollectArtifact(ctx, velociraptor.CollectionRequest{
				ClientID:   in.ClientID,
				Artifact:   artifact.Name,
				Parameters: artifact.Parameters,
			})
			if err != nil {
				flows = append(flows, CollectedFlow{Artifact: artifact.Name, Error: err.Error()})
				firstErr = err
				break
			}
			flows = append(flows, CollectedFlow{Artifact: artifact.Name, FlowID: summary.FlowID, State: string(summary.State)})
		}

		if firstErr != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference, RowCount: len(flows), Reason: firstErr.Error()})
			return nil, CollectDFIRProfileOutput{
				Result:   response.Error(fmt.Sprintf("profile collection stopped after %d/%d artifacts: %v", len(flows)-1, len(profile.Artifacts), firstErr)),
				ClientID: in.ClientID,
				Profile:  in.Profile,
				Flows:    flows,
			}, nil
		}

		recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, Profile: in.Profile, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference, RowCount: len(flows)})
		return nil, CollectDFIRProfileOutput{
			Result:   response.Result{Status: response.StatusSuccess},
			ClientID: in.ClientID,
			Profile:  in.Profile,
			Flows:    flows,
		}, nil
	}
}

// CancelFlowInput is velo_cancel_flow_with_approval's argument shape.
type CancelFlowInput struct {
	ClientID          string `json:"client_id" jsonschema:"Velociraptor client ID, e.g. C.1234abcd5678ef90"`
	FlowID            string `json:"flow_id" jsonschema:"Velociraptor flow ID to cancel, e.g. F.BN2HJC4N4T6KG"`
	CaseID            string `json:"case_id" jsonschema:"investigation/case identifier this cancellation is tied to"`
	Reason            string `json:"reason" jsonschema:"justification for why this cancellation is needed"`
	Requester         string `json:"requester" jsonschema:"identity of whoever is asking for this cancellation"`
	ApprovalReference string `json:"approval_reference" jsonschema:"reference to a pre-approved request created via the approve CLI subcommand"`
}

// CancelFlowOutput reports the outcome of a flow cancellation attempt
// via the v0.2.0 response.Result envelope.
type CancelFlowOutput struct {
	response.Result
	ClientID string `json:"client_id,omitempty"`
	FlowID   string `json:"flow_id,omitempty"`
}

func newCancelFlowHandler(deps Deps) mcp.ToolHandlerFor[CancelFlowInput, CancelFlowOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in CancelFlowInput) (*mcp.CallToolResult, CancelFlowOutput, error) {
		const tool = "velo_cancel_flow_with_approval"

		if enabled, reason := writePilotEnabled(deps); !enabled {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, Reason: reason})
			return nil, CancelFlowOutput{}, errors.New(reason)
		}

		if err := validateApprovalFields(in.ClientID, in.CaseID, in.Reason, in.Requester, in.ApprovalReference); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, Reason: err.Error()})
			return nil, CancelFlowOutput{}, err
		}
		if err := validation.FlowID(in.FlowID); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, Reason: "invalid flow id syntax"})
			return nil, CancelFlowOutput{}, fmt.Errorf("invalid flow id %q", in.FlowID)
		}

		candidate := approval.Request{
			Operation: approval.OperationCancelFlow,
			CaseID:    in.CaseID,
			ClientID:  in.ClientID,
			FlowID:    in.FlowID,
		}
		result, outcome, ok := verifyAndConsumeApproval(ctx, deps, in.ApprovalReference, candidate)
		if !ok {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: outcome, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference, Reason: result.Message})
			return nil, CancelFlowOutput{Result: result, ClientID: in.ClientID, FlowID: in.FlowID}, nil
		}

		if deps.WriteClient == nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, ApprovalID: in.ApprovalReference, Reason: "no Velociraptor write client is configured"})
			return nil, CancelFlowOutput{
				Result:   response.Error("real mode is configured but no Velociraptor write client is available"),
				ClientID: in.ClientID,
				FlowID:   in.FlowID,
			}, nil
		}

		if err := deps.WriteClient.CancelFlow(ctx, in.ClientID, in.FlowID); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, ApprovalID: in.ApprovalReference, Reason: err.Error()})
			return nil, CancelFlowOutput{
				Result:   response.Error(err.Error()),
				ClientID: in.ClientID,
				FlowID:   in.FlowID,
			}, nil
		}

		recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference})
		return nil, CancelFlowOutput{
			Result:   response.Result{Status: response.StatusSuccess},
			ClientID: in.ClientID,
			FlowID:   in.FlowID,
		}, nil
	}
}
