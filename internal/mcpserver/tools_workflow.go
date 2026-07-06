package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/dfir"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/validation"
)

// WorkflowTools are v0.3.0's read-only analyst workflow helpers. They
// operate only on the already-loaded DFIR profile registry and local
// policy allowlists: no Velociraptor RPC is made, no collection/hunt is
// started, and no endpoint/server state is mutated.
var WorkflowTools = []ToolSpec{
	{
		Name:        "velo_plan_dfir_triage",
		Description: "Recommend read-only DFIR profile workflow steps for a case type and target OS without collecting anything.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_compare_dfir_profiles",
		Description: "Compare loaded DFIR profiles by metadata and artifact overlap without executing them.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_find_profiles_by_artifact",
		Description: "Find loaded DFIR profiles that reference a given artifact name without querying endpoints.",
		ReadOnly:    true,
	},
}

// PlanDFIRTriageInput describes a planning request. Every field is
// optional: an empty request returns a broad set of loaded profiles. When
// supplied, ClientID is validated only so callers can carry a known target
// through the plan; this tool never contacts that client.
type PlanDFIRTriageInput struct {
	CaseType string `json:"case_type,omitempty" jsonschema:"optional case type such as basic, ransomware, persistence, credential_theft, lateral_movement, eventlog, browser_activity, timeline, process_network, or ioc"`
	TargetOS string `json:"target_os,omitempty" jsonschema:"optional target OS filter: windows, linux, or any"`
	ClientID string `json:"client_id,omitempty" jsonschema:"optional Velociraptor client ID to include in the plan; validated but never contacted"`
}

type WorkflowProfileRecommendation struct {
	Name                 string   `json:"name"`
	DisplayName          string   `json:"display_name"`
	Description          string   `json:"description"`
	TargetOS             string   `json:"target_os"`
	Category             string   `json:"category"`
	RiskLevel            string   `json:"risk_level"`
	RequiresApproval     bool     `json:"requires_approval"`
	ArtifactCount        int      `json:"artifact_count"`
	AllowedByPolicy      bool     `json:"allowed_by_policy"`
	ArtifactsAllowlisted bool     `json:"artifacts_allowlisted"`
	ValidationError      string   `json:"validation_error,omitempty"`
	Reasons              []string `json:"reasons"`
}

type PlanDFIRTriageOutput struct {
	response.Result
	CaseType          string                          `json:"case_type,omitempty"`
	TargetOS          string                          `json:"target_os,omitempty"`
	ClientID          string                          `json:"client_id,omitempty"`
	Recommendations   []WorkflowProfileRecommendation `json:"recommendations"`
	ReadOnlyNextSteps []string                        `json:"read_only_next_steps"`
	SafetyNotes       []string                        `json:"safety_notes"`
}

type CompareDFIRProfilesInput struct {
	Names []string `json:"names" jsonschema:"two to five DFIR profile names to compare"`
}

type ComparedProfileOutput struct {
	Name                 string   `json:"name"`
	DisplayName          string   `json:"display_name"`
	TargetOS             string   `json:"target_os"`
	Category             string   `json:"category"`
	RiskLevel            string   `json:"risk_level"`
	RequiresApproval     bool     `json:"requires_approval"`
	AllowedByPolicy      bool     `json:"allowed_by_policy"`
	ArtifactsAllowlisted bool     `json:"artifacts_allowlisted"`
	ValidationError      string   `json:"validation_error,omitempty"`
	Artifacts            []string `json:"artifacts"`
}

type CompareDFIRProfilesOutput struct {
	response.Result
	Profiles        []ComparedProfileOutput `json:"profiles"`
	CommonArtifacts []string                `json:"common_artifacts"`
	UniqueArtifacts map[string][]string     `json:"unique_artifacts"`
	MissingProfiles []string                `json:"missing_profiles,omitempty"`
}

type FindProfilesByArtifactInput struct {
	Artifact string `json:"artifact" jsonschema:"Velociraptor artifact name, e.g. Windows.System.Pslist"`
}

type FindProfilesByArtifactOutput struct {
	response.Result
	Artifact        string                          `json:"artifact"`
	ArtifactAllowed bool                            `json:"artifact_allowed"`
	Profiles        []WorkflowProfileRecommendation `json:"profiles"`
}

func registerWorkflowTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_plan_dfir_triage",
		Description: WorkflowTools[0].Description,
		Annotations: readOnlyAnnotations("Plan DFIR triage workflow"),
	}, newPlanDFIRTriageHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_compare_dfir_profiles",
		Description: WorkflowTools[1].Description,
		Annotations: readOnlyAnnotations("Compare DFIR profiles"),
	}, newCompareDFIRProfilesHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_find_profiles_by_artifact",
		Description: WorkflowTools[2].Description,
		Annotations: readOnlyAnnotations("Find DFIR profiles by artifact"),
	}, newFindProfilesByArtifactHandler(deps))
}

func newPlanDFIRTriageHandler(deps Deps) mcp.ToolHandlerFor[PlanDFIRTriageInput, PlanDFIRTriageOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in PlanDFIRTriageInput) (*mcp.CallToolResult, PlanDFIRTriageOutput, error) {
		caseType, err := normalizeWorkflowToken(in.CaseType, allowedWorkflowCaseTypes(), "case_type")
		if err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_plan_dfir_triage", Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Reason: err.Error()})
			return nil, PlanDFIRTriageOutput{}, err
		}
		targetOS, err := normalizeWorkflowToken(in.TargetOS, map[string]bool{"": true, "windows": true, "linux": true, "any": true}, "target_os")
		if err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_plan_dfir_triage", Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Reason: err.Error()})
			return nil, PlanDFIRTriageOutput{}, err
		}
		if in.ClientID != "" {
			if err := validation.ClientID(in.ClientID); err != nil {
				recordAudit(deps, audit.Event{Tool: "velo_plan_dfir_triage", Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Reason: "invalid client id syntax"})
				return nil, PlanDFIRTriageOutput{}, fmt.Errorf("invalid client id %q", in.ClientID)
			}
		}
		if deps.Profiles == nil || deps.Policy == nil {
			recordAudit(deps, audit.Event{Tool: "velo_plan_dfir_triage", Outcome: audit.OutcomeError, ClientID: in.ClientID, Reason: "dfir profile registry or policy engine not configured"})
			return nil, PlanDFIRTriageOutput{}, fmt.Errorf("dfir profile registry or policy engine is not configured")
		}

		recommendations := make([]WorkflowProfileRecommendation, 0)
		for _, p := range deps.Profiles.List() {
			reasons := workflowMatchReasons(p, caseType, targetOS)
			if len(reasons) == 0 {
				continue
			}
			recommendations = append(recommendations, workflowRecommendation(deps, p, reasons))
		}

		status := response.StatusForCount(len(recommendations))
		result := response.Result{Status: status}
		if status == response.StatusEmpty {
			result.Message = "no loaded DFIR profile matched the requested case_type and target_os filters"
		}
		recordAudit(deps, audit.Event{Tool: "velo_plan_dfir_triage", Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, RowCount: len(recommendations)})
		return nil, PlanDFIRTriageOutput{
			Result:            result,
			CaseType:          caseType,
			TargetOS:          targetOS,
			ClientID:          in.ClientID,
			Recommendations:   recommendations,
			ReadOnlyNextSteps: readOnlyWorkflowSteps(in.ClientID),
			SafetyNotes:       workflowSafetyNotes(),
		}, nil
	}
}

func newCompareDFIRProfilesHandler(deps Deps) mcp.ToolHandlerFor[CompareDFIRProfilesInput, CompareDFIRProfilesOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in CompareDFIRProfilesInput) (*mcp.CallToolResult, CompareDFIRProfilesOutput, error) {
		if len(in.Names) < 2 || len(in.Names) > 5 {
			recordAudit(deps, audit.Event{Tool: "velo_compare_dfir_profiles", Outcome: audit.OutcomeBlocked, Reason: "profile comparison requires two to five names"})
			return nil, CompareDFIRProfilesOutput{}, fmt.Errorf("profile comparison requires two to five names")
		}
		if deps.Profiles == nil || deps.Policy == nil {
			recordAudit(deps, audit.Event{Tool: "velo_compare_dfir_profiles", Outcome: audit.OutcomeError, Reason: "dfir profile registry or policy engine not configured"})
			return nil, CompareDFIRProfilesOutput{}, fmt.Errorf("dfir profile registry or policy engine is not configured")
		}

		seenNames := make(map[string]bool, len(in.Names))
		profiles := make([]dfir.Profile, 0, len(in.Names))
		missing := make([]string, 0)
		for _, rawName := range in.Names {
			name := strings.TrimSpace(rawName)
			if err := validation.DFIRProfileName(name); err != nil {
				recordAudit(deps, audit.Event{Tool: "velo_compare_dfir_profiles", Outcome: audit.OutcomeBlocked, Profile: name, Reason: "invalid profile name syntax"})
				return nil, CompareDFIRProfilesOutput{}, fmt.Errorf("invalid dfir profile name %q", name)
			}
			if seenNames[name] {
				recordAudit(deps, audit.Event{Tool: "velo_compare_dfir_profiles", Outcome: audit.OutcomeBlocked, Profile: name, Reason: "duplicate profile name"})
				return nil, CompareDFIRProfilesOutput{}, fmt.Errorf("duplicate dfir profile name %q", name)
			}
			seenNames[name] = true
			p, ok := deps.Profiles.Get(name)
			if !ok {
				missing = append(missing, name)
				continue
			}
			profiles = append(profiles, p)
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			recordAudit(deps, audit.Event{Tool: "velo_compare_dfir_profiles", Outcome: audit.OutcomeError, Reason: "one or more profiles not found"})
			return nil, CompareDFIRProfilesOutput{
				Result:          response.NotFound("one or more requested DFIR profiles were not found"),
				Profiles:        []ComparedProfileOutput{},
				CommonArtifacts: []string{},
				UniqueArtifacts: map[string][]string{},
				MissingProfiles: missing,
			}, nil
		}

		outProfiles := make([]ComparedProfileOutput, 0, len(profiles))
		artifactSets := make(map[string]map[string]bool, len(profiles))
		artifactCounts := make(map[string]int)
		for _, p := range profiles {
			artifacts := profileArtifactNames(p)
			set := make(map[string]bool, len(artifacts))
			for _, name := range artifacts {
				set[name] = true
				artifactCounts[name]++
			}
			artifactSets[p.Name] = set
			valid, validationErr := profilePolicyValidation(deps, p)
			outProfiles = append(outProfiles, ComparedProfileOutput{
				Name:                 p.Name,
				DisplayName:          p.DisplayName,
				TargetOS:             p.TargetOS,
				Category:             p.Category,
				RiskLevel:            p.RiskLevel,
				RequiresApproval:     p.RequiresApproval,
				AllowedByPolicy:      deps.Policy.ProfileAllowed(p.Name),
				ArtifactsAllowlisted: valid,
				ValidationError:      validationErr,
				Artifacts:            artifacts,
			})
		}
		common := make([]string, 0)
		unique := make(map[string][]string, len(profiles))
		for _, p := range profiles {
			for artifact := range artifactSets[p.Name] {
				if artifactCounts[artifact] == len(profiles) {
					common = append(common, artifact)
				}
				if artifactCounts[artifact] == 1 {
					unique[p.Name] = append(unique[p.Name], artifact)
				}
			}
			sort.Strings(unique[p.Name])
		}
		sort.Strings(common)

		recordAudit(deps, audit.Event{Tool: "velo_compare_dfir_profiles", Outcome: audit.OutcomeSuccess, RowCount: len(outProfiles)})
		return nil, CompareDFIRProfilesOutput{
			Result:          response.Result{Status: response.StatusSuccess},
			Profiles:        outProfiles,
			CommonArtifacts: common,
			UniqueArtifacts: unique,
		}, nil
	}
}

func newFindProfilesByArtifactHandler(deps Deps) mcp.ToolHandlerFor[FindProfilesByArtifactInput, FindProfilesByArtifactOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in FindProfilesByArtifactInput) (*mcp.CallToolResult, FindProfilesByArtifactOutput, error) {
		if err := validation.ArtifactName(in.Artifact); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_find_profiles_by_artifact", Outcome: audit.OutcomeBlocked, Artifact: in.Artifact, Reason: "invalid artifact name syntax"})
			return nil, FindProfilesByArtifactOutput{}, fmt.Errorf("invalid artifact name %q", in.Artifact)
		}
		if deps.Profiles == nil || deps.Policy == nil {
			recordAudit(deps, audit.Event{Tool: "velo_find_profiles_by_artifact", Outcome: audit.OutcomeError, Artifact: in.Artifact, Reason: "dfir profile registry or policy engine not configured"})
			return nil, FindProfilesByArtifactOutput{}, fmt.Errorf("dfir profile registry or policy engine is not configured")
		}

		matches := make([]WorkflowProfileRecommendation, 0)
		for _, p := range deps.Profiles.List() {
			for _, artifact := range p.Artifacts {
				if artifact.Name == in.Artifact {
					matches = append(matches, workflowRecommendation(deps, p, []string{"profile references requested artifact"}))
					break
				}
			}
		}
		result := response.Result{Status: response.StatusForCount(len(matches))}
		if result.Status == response.StatusEmpty {
			result = response.NotFound("no loaded DFIR profile references the requested artifact")
		}
		recordAudit(deps, audit.Event{Tool: "velo_find_profiles_by_artifact", Outcome: audit.OutcomeSuccess, Artifact: in.Artifact, RowCount: len(matches)})
		return nil, FindProfilesByArtifactOutput{
			Result:          result,
			Artifact:        in.Artifact,
			ArtifactAllowed: deps.Policy.ArtifactAllowed(in.Artifact),
			Profiles:        matches,
		}, nil
	}
}

func allowedWorkflowCaseTypes() map[string]bool {
	return map[string]bool{
		"": true, "basic": true, "triage": true, "process_network": true,
		"persistence": true, "lateral_movement": true, "ransomware": true,
		"credential_theft": true, "eventlog": true, "browser_activity": true,
		"timeline": true, "ioc": true,
	}
}

func normalizeWorkflowToken(value string, allowed map[string]bool, field string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	if !allowed[normalized] {
		return "", fmt.Errorf("invalid %s %q", field, value)
	}
	return normalized, nil
}

func workflowMatchReasons(p dfir.Profile, caseType, targetOS string) []string {
	if targetOS != "" && targetOS != "any" && p.TargetOS != "any" && p.TargetOS != targetOS {
		return nil
	}
	reasons := []string{}
	if targetOS != "" {
		reasons = append(reasons, "target_os matches")
	}
	if caseType == "" {
		if len(reasons) == 0 {
			reasons = append(reasons, "profile is loaded")
		}
		return reasons
	}
	searchText := strings.ToLower(strings.Join([]string{p.Name, p.DisplayName, p.Description, p.Category}, " "))
	caseNeedles := workflowCaseNeedles(caseType)
	for _, needle := range caseNeedles {
		if strings.Contains(searchText, needle) {
			reasons = append(reasons, fmt.Sprintf("matches case_type %q", caseType))
			return reasons
		}
	}
	return nil
}

func workflowCaseNeedles(caseType string) []string {
	switch caseType {
	case "basic", "triage":
		return []string{"basic", "triage"}
	case "process_network":
		return []string{"process_network", "process and network", "process", "network"}
	case "credential_theft":
		return []string{"credential", "theft"}
	case "lateral_movement":
		return []string{"lateral", "movement"}
	case "browser_activity":
		return []string{"browser"}
	default:
		return []string{caseType}
	}
}

func workflowRecommendation(deps Deps, p dfir.Profile, reasons []string) WorkflowProfileRecommendation {
	valid, validationErr := profilePolicyValidation(deps, p)
	return WorkflowProfileRecommendation{
		Name:                 p.Name,
		DisplayName:          p.DisplayName,
		Description:          p.Description,
		TargetOS:             p.TargetOS,
		Category:             p.Category,
		RiskLevel:            p.RiskLevel,
		RequiresApproval:     p.RequiresApproval,
		ArtifactCount:        len(p.Artifacts),
		AllowedByPolicy:      deps.Policy != nil && deps.Policy.ProfileAllowed(p.Name),
		ArtifactsAllowlisted: valid,
		ValidationError:      validationErr,
		Reasons:              reasons,
	}
}

func profilePolicyValidation(deps Deps, p dfir.Profile) (bool, string) {
	if deps.Policy == nil {
		return false, "policy engine is not configured"
	}
	if err := dfir.ValidateProfile(p, deps.Policy); err != nil {
		return false, err.Error()
	}
	return true, ""
}

func profileArtifactNames(p dfir.Profile) []string {
	artifacts := make([]string, 0, len(p.Artifacts))
	for _, artifact := range p.Artifacts {
		artifacts = append(artifacts, artifact.Name)
	}
	sort.Strings(artifacts)
	return artifacts
}

func readOnlyWorkflowSteps(clientID string) []string {
	steps := []string{
		"Run velo_search_clients to identify candidate endpoints if the client is not already known.",
		"Run velo_get_client_info for endpoint metadata before choosing an OS-specific profile.",
		"Run velo_list_artifact_names and velo_get_artifact_details to inspect available artifact metadata and parameters.",
		"Run velo_get_dfir_profile or velo_compare_dfir_profiles to review profile contents before any future approved collection workflow.",
	}
	if clientID != "" {
		steps[0] = "Client ID was supplied and syntactically validated; run velo_get_client_info to confirm live endpoint metadata."
	}
	return steps
}

func workflowSafetyNotes() []string {
	return []string{
		"v0.3.0 workflow tools are read-only planning helpers and do not execute collections, hunts, downloads, cancellations, raw VQL, or client-side mutation.",
		"allowed_by_policy and artifacts_allowlisted are guidance for future approved workflows; they do not grant execution capability.",
		"Any future collection or hunt remains out of scope for this release and must be implemented separately with approval gating.",
	}
}
