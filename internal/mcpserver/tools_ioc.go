package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/validation"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/vql"
)

// IOCTools expose the single IOC-hunting convenience tool. This is the
// only place a hash/IP/domain/process/path literal enters a hunt; it is
// built on top of the same approval/scope/audit machinery as HuntTools
// (verifyApproval, backendOperationReady, consumeApproval,
// validateHuntScopeInput, writePilotEnabled),
// resolves its target through vql.KnownTemplates/vql.Bind, and starts
// the hunt via the same velociraptor.HuntWriter.StartHunt path
// velo_start_hunt_with_approval uses. Never raw VQL, never a
// caller-chosen artifact name.
var IOCTools = []ToolSpec{
	{
		Name:             "velo_hunt_ioc_with_approval",
		Description:      "Hunt for a validated hash, IP, domain, process, or path indicator across a previewed, bounded scope using a fixed IOC template. Requires approval.",
		RequiresApproval: true,
	},
}

// iocTemplateForKind maps a validated IOC kind to its fixed vql
// template. This mapping, like vql.KnownTemplates itself, is the
// allowlist: an IOCKind with no entry here can never reach vql.Bind.
func iocTemplateForKind(kind validation.IOCKind) (vql.TemplateName, bool) {
	switch kind {
	case validation.IOCKindHash:
		return vql.TemplateIOCHashHunt, true
	case validation.IOCKindIP:
		return vql.TemplateIOCIPHunt, true
	case validation.IOCKindDomain:
		return vql.TemplateIOCDomainHunt, true
	case validation.IOCKindProcess:
		return vql.TemplateIOCProcessHunt, true
	case validation.IOCKindPath:
		return vql.TemplateIOCPathHunt, true
	default:
		return "", false
	}
}

// HuntIOCInput is velo_hunt_ioc_with_approval's argument shape. Kind and
// Value are validated together via validation.ValidateIOC before Value
// is ever bound into a template parameter.
type HuntIOCInput struct {
	CaseID     string   `json:"case_id" jsonschema:"investigation case ID (required)"`
	Reason     string   `json:"reason" jsonschema:"justification for hunting this indicator (required)"`
	Requester  string   `json:"requester" jsonschema:"person requesting the hunt (required)"`
	ApprovalID string   `json:"approval_id" jsonschema:"approval reference ID (required)"`
	Kind       string   `json:"kind" jsonschema:"ioc kind: hash, ip, domain, process, or path"`
	Value      string   `json:"value" jsonschema:"the indicator value to hunt for, e.g. a file hash, IP, domain, process name, or path"`
	ClientIDs  []string `json:"client_ids,omitempty" jsonschema:"explicit client IDs to target"`
	Label      string   `json:"label,omitempty" jsonschema:"label filter"`
	All        bool     `json:"all,omitempty" jsonschema:"target all clients; blocked by default unless policy allows"`
	MaxClients int      `json:"max_clients,omitempty" jsonschema:"max clients cap"`
}

// HuntIOCOutput reports the outcome of an IOC hunt attempt via the
// v0.2.0 response.Result envelope.
type HuntIOCOutput struct {
	response.Result
	Mode      string `json:"mode"`
	HuntID    string `json:"hunt_id,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Artifact  string `json:"artifact,omitempty"`
	State     string `json:"state,omitempty"`
	ScopeDesc string `json:"scope_desc,omitempty"`
}

func newHuntIOCHandler(deps Deps) mcp.ToolHandlerFor[HuntIOCInput, HuntIOCOutput] {
	const tool = "velo_hunt_ioc_with_approval"

	return func(ctx context.Context, req *mcp.CallToolRequest, in HuntIOCInput) (*mcp.CallToolResult, HuntIOCOutput, error) {
		if enabled, reason := writePilotEnabled(deps); !enabled {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, CaseID: in.CaseID, IOCKind: in.Kind, Reason: reason})
			return nil, HuntIOCOutput{}, errors.New(reason)
		}
		if err := validateHuntWriteInput(deps, in.CaseID, in.Reason, in.ApprovalID); err != nil {
			return nil, HuntIOCOutput{}, err
		}
		if err := validateHuntScopeInput(in.ClientIDs, in.Label, in.All); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, CaseID: in.CaseID, IOCKind: in.Kind, Reason: err.Error()})
			return nil, HuntIOCOutput{}, err
		}

		kind := validation.IOCKind(in.Kind)
		if err := validation.ValidateIOC(kind, in.Value); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, CaseID: in.CaseID, IOCKind: in.Kind, Reason: err.Error()})
			return nil, HuntIOCOutput{}, err
		}

		template, ok := iocTemplateForKind(kind)
		if !ok {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, CaseID: in.CaseID, IOCKind: in.Kind, Reason: "unsupported ioc kind"})
			return nil, HuntIOCOutput{}, fmt.Errorf("unsupported ioc kind %q", in.Kind)
		}

		artifact, params, err := vql.Bind(template, vql.Env{"value": in.Value})
		if err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, CaseID: in.CaseID, IOCKind: in.Kind, Reason: err.Error()})
			return nil, HuntIOCOutput{}, err
		}

		if deps.Policy != nil && !deps.Policy.ArtifactAllowed(artifact) {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, CaseID: in.CaseID, Artifact: artifact, IOCKind: in.Kind, Reason: "artifact not in allowlist"})
			return nil, HuntIOCOutput{}, fmt.Errorf("artifact %q (resolved from ioc template) is not in the configured allowlist", artifact)
		}

		if in.All && (deps.Policy == nil || !deps.Policy.TargetAllAllowed()) {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, CaseID: in.CaseID, Artifact: artifact, IOCKind: in.Kind, Reason: "target_all is disabled by policy"})
			return nil, HuntIOCOutput{}, fmt.Errorf("target_all is disabled by policy")
		}

		maxClients := configuredMaxHuntClients(deps)
		if in.MaxClients > 0 && in.MaxClients < maxClients {
			maxClients = in.MaxClients
		}

		candidate := approval.Request{
			Operation:  approval.OperationHuntIOC,
			CaseID:     in.CaseID,
			Artifact:   artifact,
			Parameters: params,
			ClientIDs:  in.ClientIDs,
			Label:      in.Label,
			TargetAll:  in.All,
		}
		result, outcome, ok := verifyApproval(ctx, deps, in.ApprovalID, candidate)
		if !ok {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: outcome, CaseID: in.CaseID, Artifact: artifact, IOCKind: in.Kind, IOCValue: in.Value, RequestReason: in.Reason, ApprovalID: in.ApprovalID, Reason: result.Message})
			return nil, HuntIOCOutput{Result: result, Kind: in.Kind, Artifact: artifact}, nil
		}
		if result := backendOperationReady(deps.WriteClient, velociraptor.BackendOpStartHunt); result.Status != "" {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, CaseID: in.CaseID, Artifact: artifact, IOCKind: in.Kind, IOCValue: in.Value, Reason: result.Message})
			return nil, HuntIOCOutput{Result: result, Kind: in.Kind, Artifact: artifact}, nil
		}
		result, outcome, ok = consumeApproval(ctx, deps, in.ApprovalID)
		if !ok {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: outcome, CaseID: in.CaseID, Artifact: artifact, IOCKind: in.Kind, IOCValue: in.Value, RequestReason: in.Reason, ApprovalID: in.ApprovalID, Reason: result.Message})
			return nil, HuntIOCOutput{Result: result, Kind: in.Kind, Artifact: artifact}, nil
		}

		hunt, err := deps.WriteClient.StartHunt(ctx, velociraptor.HuntRequest{
			Artifact:   artifact,
			Parameters: params,
			Scope: velociraptor.HuntScopeRequest{
				ClientIDs: in.ClientIDs,
				Label:     in.Label,
				All:       in.All,
			},
			MaxClients: maxClients,
		})
		if err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, CaseID: in.CaseID, Artifact: artifact, IOCKind: in.Kind, IOCValue: in.Value, ApprovalID: in.ApprovalID, Reason: err.Error()})
			return nil, HuntIOCOutput{Result: response.Error(err.Error()), Mode: VelociraptorModeReal, Kind: in.Kind, Artifact: artifact}, nil
		}

		scopeDesc := describeScope(in.ClientIDs, in.Label, in.All)
		recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeSuccess, HuntID: hunt.HuntID, Artifact: artifact, IOCKind: in.Kind, IOCValue: in.Value, CaseID: in.CaseID, ApprovalID: in.ApprovalID})
		return nil, HuntIOCOutput{
			Result:    response.Success("ioc hunt started"),
			Mode:      VelociraptorModeReal,
			HuntID:    hunt.HuntID,
			Kind:      in.Kind,
			Artifact:  artifact,
			State:     string(hunt.State),
			ScopeDesc: scopeDesc,
		}, nil
	}
}

func registerIOCTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_hunt_ioc_with_approval",
		Description: IOCTools[0].Description,
		Annotations: writeAnnotations("Hunt IOC (approval-gated)"),
	}, newHuntIOCHandler(deps))
}
