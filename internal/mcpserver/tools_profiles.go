package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/dfir"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/validation"
)

// ProfileTools expose the DFIR profile catalog itself. All read-only:
// browsing and validating a profile definition never touches an
// endpoint. All three are registered with the MCP server as of
// v0.1.0-alpha.1 (see registerProfileTools); they only read from
// deps.Profiles (internal/dfir.Registry) and, for validation,
// deps.Policy's artifact allowlist. No Velociraptor call is made.
var ProfileTools = []ToolSpec{
	{
		Name:        "velo_list_dfir_profiles",
		Description: "List allowlisted DFIR profiles with their target OS and category.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_dfir_profile",
		Description: "Return the full definition (artifact list) of one allowlisted DFIR profile.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_validate_dfir_profile",
		Description: "Check a DFIR profile definition against the current artifact allowlist and schema rules without collecting anything.",
		ReadOnly:    true,
	},
}

// ProfileArtifactOutput mirrors dfir.ProfileArtifact with explicit JSON
// tags for the MCP tool response schema.
type ProfileArtifactOutput struct {
	Name       string            `json:"name"`
	Parameters map[string]string `json:"parameters,omitempty"`
}

// DFIRProfileOutput mirrors dfir.Profile with explicit JSON tags for the
// MCP tool response schema. Kept separate from dfir.Profile (which uses
// yaml tags for on-disk parsing) so the MCP-facing shape can evolve
// independently of the on-disk format.
type DFIRProfileOutput struct {
	Name              string                  `json:"name"`
	DisplayName       string                  `json:"display_name"`
	Description       string                  `json:"description"`
	TargetOS          string                  `json:"target_os"`
	Category          string                  `json:"category"`
	RiskLevel         string                  `json:"risk_level"`
	RequiresApproval  bool                    `json:"requires_approval"`
	MaxRuntimeSeconds int                     `json:"max_runtime_seconds"`
	MaxResultRows     int                     `json:"max_result_rows"`
	MaxResultBytes    int64                   `json:"max_result_bytes"`
	Artifacts         []ProfileArtifactOutput `json:"artifacts"`
}

func toProfileOutput(p dfir.Profile) DFIRProfileOutput {
	artifacts := make([]ProfileArtifactOutput, 0, len(p.Artifacts))
	for _, a := range p.Artifacts {
		artifacts = append(artifacts, ProfileArtifactOutput{Name: a.Name, Parameters: a.Parameters})
	}
	return DFIRProfileOutput{
		Name:              p.Name,
		DisplayName:       p.DisplayName,
		Description:       p.Description,
		TargetOS:          p.TargetOS,
		Category:          p.Category,
		RiskLevel:         p.RiskLevel,
		RequiresApproval:  p.RequiresApproval,
		MaxRuntimeSeconds: p.MaxRuntimeSeconds,
		MaxResultRows:     p.MaxResultRows,
		MaxResultBytes:    p.MaxResultBytes,
		Artifacts:         artifacts,
	}
}

// DFIRProfileSummary is the abbreviated form returned by
// velo_list_dfir_profiles: enough to pick a profile without transferring
// every artifact list.
type DFIRProfileSummary struct {
	Name             string `json:"name"`
	DisplayName      string `json:"display_name"`
	Description      string `json:"description"`
	TargetOS         string `json:"target_os"`
	Category         string `json:"category"`
	RiskLevel        string `json:"risk_level"`
	RequiresApproval bool   `json:"requires_approval"`
	ArtifactCount    int    `json:"artifact_count"`
}

// ListDFIRProfilesInput is empty: velo_list_dfir_profiles takes no
// arguments.
type ListDFIRProfilesInput struct{}

// ListDFIRProfilesOutput wraps the profile summary list.
type ListDFIRProfilesOutput struct {
	Profiles []DFIRProfileSummary `json:"profiles"`
}

// GetDFIRProfileInput names the profile to retrieve. The name is
// validated with internal/validation.DFIRProfileName before any registry
// lookup, so malformed input never reaches dfir.Registry.
type GetDFIRProfileInput struct {
	Name string `json:"name" jsonschema:"DFIR profile name, e.g. windows_basic_triage"`
}

// GetDFIRProfileOutput is the full profile definition.
type GetDFIRProfileOutput struct {
	Profile DFIRProfileOutput `json:"profile"`
}

// ValidateDFIRProfileInput names the profile to validate.
type ValidateDFIRProfileInput struct {
	Name string `json:"name" jsonschema:"DFIR profile name, e.g. windows_basic_triage"`
}

// ValidateDFIRProfileOutput reports whether the named profile's
// artifacts are all present in the current policy artifact allowlist.
// Valid=false is a normal, informative result, not a tool call failure:
// it means the profile is currently unusable for collection/hunting
// until the allowlist or profile is updated, which is exactly what an
// operator running this tool wants to know.
type ValidateDFIRProfileOutput struct {
	Name  string `json:"name"`
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

func registerProfileTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_list_dfir_profiles",
		Description: ProfileTools[0].Description,
		Annotations: readOnlyAnnotations("List DFIR profiles"),
	}, newListDFIRProfilesHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_get_dfir_profile",
		Description: ProfileTools[1].Description,
		Annotations: readOnlyAnnotations("Get DFIR profile"),
	}, newGetDFIRProfileHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_validate_dfir_profile",
		Description: ProfileTools[2].Description,
		Annotations: readOnlyAnnotations("Validate DFIR profile"),
	}, newValidateDFIRProfileHandler(deps))
}

func newListDFIRProfilesHandler(deps Deps) mcp.ToolHandlerFor[ListDFIRProfilesInput, ListDFIRProfilesOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ListDFIRProfilesInput) (*mcp.CallToolResult, ListDFIRProfilesOutput, error) {
		if deps.Profiles == nil {
			recordAudit(deps, audit.Event{
				Tool:    "velo_list_dfir_profiles",
				Outcome: audit.OutcomeError,
				Reason:  "dfir profile registry not configured",
			})
			return nil, ListDFIRProfilesOutput{}, fmt.Errorf("dfir profile registry is not configured")
		}

		loaded := deps.Profiles.List()
		summaries := make([]DFIRProfileSummary, 0, len(loaded))
		for _, p := range loaded {
			summaries = append(summaries, DFIRProfileSummary{
				Name:             p.Name,
				DisplayName:      p.DisplayName,
				Description:      p.Description,
				TargetOS:         p.TargetOS,
				Category:         p.Category,
				RiskLevel:        p.RiskLevel,
				RequiresApproval: p.RequiresApproval,
				ArtifactCount:    len(p.Artifacts),
			})
		}

		recordAudit(deps, audit.Event{
			Tool:     "velo_list_dfir_profiles",
			Outcome:  audit.OutcomeSuccess,
			RowCount: len(summaries),
		})

		return nil, ListDFIRProfilesOutput{Profiles: summaries}, nil
	}
}

func newGetDFIRProfileHandler(deps Deps) mcp.ToolHandlerFor[GetDFIRProfileInput, GetDFIRProfileOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in GetDFIRProfileInput) (*mcp.CallToolResult, GetDFIRProfileOutput, error) {
		if err := validation.DFIRProfileName(in.Name); err != nil {
			recordAudit(deps, audit.Event{
				Tool:    "velo_get_dfir_profile",
				Outcome: audit.OutcomeBlocked,
				Profile: in.Name,
				Reason:  "invalid profile name syntax",
			})
			return nil, GetDFIRProfileOutput{}, fmt.Errorf("invalid dfir profile name %q", in.Name)
		}

		if deps.Profiles == nil {
			recordAudit(deps, audit.Event{
				Tool:    "velo_get_dfir_profile",
				Outcome: audit.OutcomeError,
				Profile: in.Name,
				Reason:  "dfir profile registry not configured",
			})
			return nil, GetDFIRProfileOutput{}, fmt.Errorf("dfir profile registry is not configured")
		}

		p, ok := deps.Profiles.Get(in.Name)
		if !ok {
			recordAudit(deps, audit.Event{
				Tool:    "velo_get_dfir_profile",
				Outcome: audit.OutcomeError,
				Profile: in.Name,
				Reason:  "profile not found",
			})
			return nil, GetDFIRProfileOutput{}, fmt.Errorf("dfir profile %q not found", in.Name)
		}

		recordAudit(deps, audit.Event{
			Tool:    "velo_get_dfir_profile",
			Outcome: audit.OutcomeSuccess,
			Profile: in.Name,
		})

		return nil, GetDFIRProfileOutput{Profile: toProfileOutput(p)}, nil
	}
}

func newValidateDFIRProfileHandler(deps Deps) mcp.ToolHandlerFor[ValidateDFIRProfileInput, ValidateDFIRProfileOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ValidateDFIRProfileInput) (*mcp.CallToolResult, ValidateDFIRProfileOutput, error) {
		if err := validation.DFIRProfileName(in.Name); err != nil {
			recordAudit(deps, audit.Event{
				Tool:    "velo_validate_dfir_profile",
				Outcome: audit.OutcomeBlocked,
				Profile: in.Name,
				Reason:  "invalid profile name syntax",
			})
			return nil, ValidateDFIRProfileOutput{}, fmt.Errorf("invalid dfir profile name %q", in.Name)
		}

		if deps.Profiles == nil || deps.Policy == nil {
			recordAudit(deps, audit.Event{
				Tool:    "velo_validate_dfir_profile",
				Outcome: audit.OutcomeError,
				Profile: in.Name,
				Reason:  "dfir profile registry or policy engine not configured",
			})
			return nil, ValidateDFIRProfileOutput{}, fmt.Errorf("dfir profile registry or policy engine is not configured")
		}

		p, ok := deps.Profiles.Get(in.Name)
		if !ok {
			recordAudit(deps, audit.Event{
				Tool:    "velo_validate_dfir_profile",
				Outcome: audit.OutcomeError,
				Profile: in.Name,
				Reason:  "profile not found",
			})
			return nil, ValidateDFIRProfileOutput{}, fmt.Errorf("dfir profile %q not found", in.Name)
		}

		out := ValidateDFIRProfileOutput{Name: in.Name, Valid: true}
		if err := dfir.ValidateProfile(p, deps.Policy); err != nil {
			out.Valid = false
			out.Error = err.Error()
		}

		recordAudit(deps, audit.Event{
			Tool:    "velo_validate_dfir_profile",
			Outcome: audit.OutcomeSuccess,
			Profile: in.Name,
			Reason:  out.Error,
		})

		return nil, out, nil
	}
}
