package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/validation"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

// VisibilityTools are read-only, always-on tools requiring no approval:
// health, client discovery, and artifact catalog browsing.
//
// All five are registered with the MCP server as of v0.1.0 (see
// registerVisibilityTools). Every one honestly reports mock vs. real
// mode: in mock mode (Deps.VelociraptorReadMode != "real") no
// Velociraptor call is ever made and the response says so explicitly,
// matching velo_health_check's existing "evidence honesty" behavior —
// see docs/security-model.md.
var VisibilityTools = []ToolSpec{
	{
		Name:        "velo_health_check",
		Description: "Report connectivity and identity of the configured Velociraptor read API without touching any client/endpoint state.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_search_clients",
		Description: "Search for Velociraptor clients by hostname/IP/label substring. Never accepts raw VQL.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_client_info",
		Description: "Return detail (hostname, OS, labels, last seen) for one already-identified client ID.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_list_artifact_names",
		Description: "List artifact names visible to this tool; restricted to the configured allowlist unless policy.allow_list_all_artifacts is set.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_artifact_details",
		Description: "Return the parameter schema (not VQL body) for one artifact.",
		ReadOnly:    true,
	},
}

// HealthCheckInput is empty: velo_health_check takes no arguments.
type HealthCheckInput struct{}

// HealthCheckOutput reports the outcome of a health check. Whether
// Velociraptor was actually contacted is always reported honestly via
// Mode/VelociraptorConnected — a "mock" mode response never claims
// VelociraptorConnected: true, and a "real" mode response that failed
// reports VelociraptorConnected: false with a safe Error message rather
// than silently looking like success.
//
// This is a single structured result, not a Go-level tool error, even
// when the underlying health check failed: whether Velociraptor is
// reachable is data the caller asked for, not a failure of the
// velo_health_check tool itself. See docs/security-model.md's evidence
// honesty section.
type HealthCheckOutput struct {
	Status                string `json:"status"`
	Mode                  string `json:"mode"`
	VelociraptorConnected bool   `json:"velociraptor_connected"`

	// ServerVersion is populated only when a future RPC exposes it.
	// Velociraptor's dedicated Check health RPC (used here) carries no
	// version field, so this is always empty for a "real" mode result
	// in this milestone; see internal/velociraptor/grpcclient.go.
	ServerVersion string `json:"server_version,omitempty"`

	Message string `json:"message"`
}

// registerVisibilityTools registers every VisibilityTools entry: all
// five are implemented and safe as of v0.1.0.
func registerVisibilityTools(s *mcp.Server, deps Deps) {
	title := "Velociraptor health check (mock)"
	if deps.VelociraptorReadMode == VelociraptorModeReal {
		title = "Velociraptor health check"
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_health_check",
		Description: VisibilityTools[0].Description,
		Annotations: readOnlyAnnotations(title),
	}, newHealthCheckHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_search_clients",
		Description: VisibilityTools[1].Description,
		Annotations: readOnlyAnnotations("Search Velociraptor clients"),
	}, newSearchClientsHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_get_client_info",
		Description: VisibilityTools[2].Description,
		Annotations: readOnlyAnnotations("Get Velociraptor client info"),
	}, newGetClientInfoHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_list_artifact_names",
		Description: VisibilityTools[3].Description,
		Annotations: readOnlyAnnotations("List Velociraptor artifact names"),
	}, newListArtifactNamesHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_get_artifact_details",
		Description: VisibilityTools[4].Description,
		Annotations: readOnlyAnnotations("Get Velociraptor artifact details"),
	}, newGetArtifactDetailsHandler(deps))
}

func newHealthCheckHandler(deps Deps) mcp.ToolHandlerFor[HealthCheckInput, HealthCheckOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in HealthCheckInput) (*mcp.CallToolResult, HealthCheckOutput, error) {
		if deps.VelociraptorReadMode != VelociraptorModeReal {
			out := HealthCheckOutput{
				Status:                "ok",
				Mode:                  VelociraptorModeMock,
				VelociraptorConnected: false,
				Message:               "MCP server is running in mock mode (velociraptor.read_api_config_path is not configured); no Velociraptor call was made",
			}
			recordAudit(deps, audit.Event{
				Tool:    "velo_health_check",
				Outcome: audit.OutcomeSuccess,
				Reason:  "mock mode, no Velociraptor call made",
			})
			return nil, out, nil
		}

		if deps.ReadClient == nil {
			out := HealthCheckOutput{
				Status:                "error",
				Mode:                  VelociraptorModeReal,
				VelociraptorConnected: false,
				Message:               "real mode is configured but no Velociraptor client is available",
			}
			recordAudit(deps, audit.Event{
				Tool:    "velo_health_check",
				Outcome: audit.OutcomeError,
				Reason:  "VelociraptorReadMode is real but ReadClient is nil",
			})
			return nil, out, nil
		}

		info, err := deps.ReadClient.HealthCheck(ctx)
		if err != nil {
			out := HealthCheckOutput{
				Status:                "error",
				Mode:                  VelociraptorModeReal,
				VelociraptorConnected: false,
				Message:               err.Error(),
			}
			recordAudit(deps, audit.Event{
				Tool:    "velo_health_check",
				Outcome: audit.OutcomeError,
				Reason:  err.Error(),
			})
			return nil, out, nil
		}

		out := HealthCheckOutput{
			Status:                "ok",
			Mode:                  VelociraptorModeReal,
			VelociraptorConnected: true,
			ServerVersion:         info.ServerVersion,
			Message:               "connected to Velociraptor API and received a healthy status from the Check RPC",
		}
		recordAudit(deps, audit.Event{
			Tool:    "velo_health_check",
			Outcome: audit.OutcomeSuccess,
		})
		return nil, out, nil
	}
}

// ClientSummaryOutput mirrors velociraptor.ClientSummary with explicit
// JSON tags for the MCP tool response schema.
type ClientSummaryOutput struct {
	ClientID     string `json:"client_id"`
	Hostname     string `json:"hostname,omitempty"`
	OS           string `json:"os,omitempty"`
	LastSeenAt   string `json:"last_seen_at,omitempty"`
	LastIP       string `json:"last_ip,omitempty"`
	AgentVersion string `json:"agent_version,omitempty"`
}

func toClientSummaryOutput(c velociraptor.ClientSummary) ClientSummaryOutput {
	return ClientSummaryOutput{
		ClientID:     c.ClientID,
		Hostname:     c.Hostname,
		OS:           c.OS,
		LastSeenAt:   c.LastSeenAt,
		LastIP:       c.LastIP,
		AgentVersion: c.AgentVersion,
	}
}

// SearchClientsInput is velo_search_clients' argument shape. Query is
// Velociraptor's own client-search syntax (hostname/IP/label substring
// or glob), never VQL; an empty query matches every client, still
// subject to Limit. Limit is advisory: the server always applies its own
// configured ceiling (config.VelociraptorConfig.MaxRows) regardless of
// what is requested here.
type SearchClientsInput struct {
	Query string `json:"query,omitempty" jsonschema:"optional hostname/IP/label substring or glob filter; empty matches all clients"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of clients to return; server-side ceiling applies even if omitted or too large"`
}

// SearchClientsOutput reports results plus, honestly, whether this was a
// real Velociraptor call at all (see HealthCheckOutput's doc comment for
// why Mode/Message exist even on a tool whose "normal" response is a
// data list).
type SearchClientsOutput struct {
	Mode    string                `json:"mode"`
	Clients []ClientSummaryOutput `json:"clients"`
	Message string                `json:"message,omitempty"`
}

func newSearchClientsHandler(deps Deps) mcp.ToolHandlerFor[SearchClientsInput, SearchClientsOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SearchClientsInput) (*mcp.CallToolResult, SearchClientsOutput, error) {
		if err := validation.SearchQuery(in.Query); err != nil {
			recordAudit(deps, audit.Event{
				Tool:    "velo_search_clients",
				Outcome: audit.OutcomeBlocked,
				Reason:  "invalid search query syntax",
			})
			return nil, SearchClientsOutput{}, fmt.Errorf("invalid search query: %w", err)
		}

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{
				Tool:    "velo_search_clients",
				Outcome: audit.OutcomeSuccess,
				Reason:  "mock mode, no Velociraptor call made",
			})
			return nil, SearchClientsOutput{
				Mode:    VelociraptorModeMock,
				Clients: []ClientSummaryOutput{},
				Message: "MCP server is running in mock mode (velociraptor.read_api_config_path is not configured); no Velociraptor call was made",
			}, nil
		}

		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{
				Tool:    "velo_search_clients",
				Outcome: audit.OutcomeError,
				Reason:  "VelociraptorReadMode is real but ReadClient is nil",
			})
			return nil, SearchClientsOutput{
				Mode:    VelociraptorModeReal,
				Clients: []ClientSummaryOutput{},
				Message: "real mode is configured but no Velociraptor client is available",
			}, nil
		}

		clients, err := deps.ReadClient.SearchClients(ctx, in.Query, in.Limit)
		if err != nil {
			recordAudit(deps, audit.Event{
				Tool:    "velo_search_clients",
				Outcome: audit.OutcomeError,
				Reason:  err.Error(),
			})
			return nil, SearchClientsOutput{
				Mode:    VelociraptorModeReal,
				Clients: []ClientSummaryOutput{},
				Message: err.Error(),
			}, nil
		}

		out := make([]ClientSummaryOutput, 0, len(clients))
		for _, c := range clients {
			out = append(out, toClientSummaryOutput(c))
		}

		recordAudit(deps, audit.Event{
			Tool:     "velo_search_clients",
			Outcome:  audit.OutcomeSuccess,
			RowCount: len(out),
		})

		return nil, SearchClientsOutput{Mode: VelociraptorModeReal, Clients: out}, nil
	}
}

// ClientDetailOutput mirrors velociraptor.ClientDetail with explicit
// JSON tags for the MCP tool response schema.
type ClientDetailOutput struct {
	ClientSummaryOutput
	Labels       []string `json:"labels,omitempty"`
	MacAddresses []string `json:"mac_addresses,omitempty"`
}

func toClientDetailOutput(c velociraptor.ClientDetail) ClientDetailOutput {
	return ClientDetailOutput{
		ClientSummaryOutput: toClientSummaryOutput(c.ClientSummary),
		Labels:              c.Labels,
		MacAddresses:        c.MacAddresses,
	}
}

// GetClientInfoInput names the client to retrieve. ClientID is validated
// with internal/validation.ClientID before any Velociraptor call, so
// malformed input never reaches the network.
type GetClientInfoInput struct {
	ClientID string `json:"client_id" jsonschema:"Velociraptor client ID, e.g. C.1234abcd5678ef90"`
}

// GetClientInfoOutput reports the client detail plus, honestly, mode and
// any failure message; Client is nil whenever no data was returned (mock
// mode or a failed real call).
type GetClientInfoOutput struct {
	Mode    string              `json:"mode"`
	Client  *ClientDetailOutput `json:"client,omitempty"`
	Message string              `json:"message,omitempty"`
}

func newGetClientInfoHandler(deps Deps) mcp.ToolHandlerFor[GetClientInfoInput, GetClientInfoOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in GetClientInfoInput) (*mcp.CallToolResult, GetClientInfoOutput, error) {
		if err := validation.ClientID(in.ClientID); err != nil {
			recordAudit(deps, audit.Event{
				Tool:     "velo_get_client_info",
				Outcome:  audit.OutcomeBlocked,
				ClientID: in.ClientID,
				Reason:   "invalid client id syntax",
			})
			return nil, GetClientInfoOutput{}, fmt.Errorf("invalid client id %q", in.ClientID)
		}

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{
				Tool:     "velo_get_client_info",
				Outcome:  audit.OutcomeSuccess,
				ClientID: in.ClientID,
				Reason:   "mock mode, no Velociraptor call made",
			})
			return nil, GetClientInfoOutput{
				Mode:    VelociraptorModeMock,
				Message: "MCP server is running in mock mode (velociraptor.read_api_config_path is not configured); no Velociraptor call was made",
			}, nil
		}

		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{
				Tool:     "velo_get_client_info",
				Outcome:  audit.OutcomeError,
				ClientID: in.ClientID,
				Reason:   "VelociraptorReadMode is real but ReadClient is nil",
			})
			return nil, GetClientInfoOutput{
				Mode:    VelociraptorModeReal,
				Message: "real mode is configured but no Velociraptor client is available",
			}, nil
		}

		detail, err := deps.ReadClient.GetClientInfo(ctx, in.ClientID)
		if err != nil {
			recordAudit(deps, audit.Event{
				Tool:     "velo_get_client_info",
				Outcome:  audit.OutcomeError,
				ClientID: in.ClientID,
				Reason:   err.Error(),
			})
			return nil, GetClientInfoOutput{
				Mode:    VelociraptorModeReal,
				Message: err.Error(),
			}, nil
		}

		out := toClientDetailOutput(detail)
		recordAudit(deps, audit.Event{
			Tool:     "velo_get_client_info",
			Outcome:  audit.OutcomeSuccess,
			ClientID: in.ClientID,
		})

		return nil, GetClientInfoOutput{Mode: VelociraptorModeReal, Client: &out}, nil
	}
}

// ArtifactSummaryOutput mirrors velociraptor.ArtifactSummary with
// explicit JSON tags for the MCP tool response schema.
type ArtifactSummaryOutput struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func toArtifactSummaryOutput(a velociraptor.ArtifactSummary) ArtifactSummaryOutput {
	return ArtifactSummaryOutput{Name: a.Name, Description: a.Description}
}

// ListArtifactNamesInput is empty: velo_list_artifact_names takes no
// arguments.
type ListArtifactNamesInput struct{}

// ListArtifactNamesOutput reports the artifact catalog (allowlist-scoped
// by default; see newListArtifactNamesHandler) plus, honestly, mode and
// any failure message.
type ListArtifactNamesOutput struct {
	Mode      string                  `json:"mode"`
	Artifacts []ArtifactSummaryOutput `json:"artifacts"`
	Message   string                  `json:"message,omitempty"`
}

// filterAllowedArtifacts restricts summaries to deps.Policy's artifact
// allowlist unless AllowListAllArtifacts is set, per ArtifactReader's
// doc comment in internal/velociraptor/artifacts.go. A nil deps.Policy
// is treated as "no policy configured" and fails closed to the
// allowlist (i.e. filters everything out), never open.
func filterAllowedArtifacts(deps Deps, all []velociraptor.ArtifactSummary) []velociraptor.ArtifactSummary {
	if deps.Policy != nil && deps.Policy.AllowListAllArtifacts() {
		return all
	}
	out := make([]velociraptor.ArtifactSummary, 0, len(all))
	for _, a := range all {
		if deps.Policy != nil && deps.Policy.ArtifactAllowed(a.Name) {
			out = append(out, a)
		}
	}
	return out
}

func newListArtifactNamesHandler(deps Deps) mcp.ToolHandlerFor[ListArtifactNamesInput, ListArtifactNamesOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ListArtifactNamesInput) (*mcp.CallToolResult, ListArtifactNamesOutput, error) {
		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{
				Tool:    "velo_list_artifact_names",
				Outcome: audit.OutcomeSuccess,
				Reason:  "mock mode, no Velociraptor call made",
			})
			return nil, ListArtifactNamesOutput{
				Mode:      VelociraptorModeMock,
				Artifacts: []ArtifactSummaryOutput{},
				Message:   "MCP server is running in mock mode (velociraptor.read_api_config_path is not configured); no Velociraptor call was made",
			}, nil
		}

		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{
				Tool:    "velo_list_artifact_names",
				Outcome: audit.OutcomeError,
				Reason:  "VelociraptorReadMode is real but ReadClient is nil",
			})
			return nil, ListArtifactNamesOutput{
				Mode:      VelociraptorModeReal,
				Artifacts: []ArtifactSummaryOutput{},
				Message:   "real mode is configured but no Velociraptor client is available",
			}, nil
		}

		artifacts, err := deps.ReadClient.ListArtifactNames(ctx)
		if err != nil {
			recordAudit(deps, audit.Event{
				Tool:    "velo_list_artifact_names",
				Outcome: audit.OutcomeError,
				Reason:  err.Error(),
			})
			return nil, ListArtifactNamesOutput{
				Mode:      VelociraptorModeReal,
				Artifacts: []ArtifactSummaryOutput{},
				Message:   err.Error(),
			}, nil
		}

		allowed := filterAllowedArtifacts(deps, artifacts)
		out := make([]ArtifactSummaryOutput, 0, len(allowed))
		for _, a := range allowed {
			out = append(out, toArtifactSummaryOutput(a))
		}

		recordAudit(deps, audit.Event{
			Tool:     "velo_list_artifact_names",
			Outcome:  audit.OutcomeSuccess,
			RowCount: len(out),
		})

		return nil, ListArtifactNamesOutput{Mode: VelociraptorModeReal, Artifacts: out}, nil
	}
}

// ArtifactParameterOutput mirrors velociraptor.ArtifactParameter with
// explicit JSON tags for the MCP tool response schema.
type ArtifactParameterOutput struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description,omitempty"`
	Default     string `json:"default,omitempty"`
}

// ArtifactDetailOutput mirrors velociraptor.ArtifactDetail. It
// deliberately has no field for an artifact's VQL body: see
// velociraptor.ArtifactDetail's doc comment and
// docs/security-model.md.
type ArtifactDetailOutput struct {
	ArtifactSummaryOutput
	Parameters []ArtifactParameterOutput `json:"parameters,omitempty"`
}

func toArtifactDetailOutput(a velociraptor.ArtifactDetail) ArtifactDetailOutput {
	params := make([]ArtifactParameterOutput, 0, len(a.Parameters))
	for _, p := range a.Parameters {
		params = append(params, ArtifactParameterOutput{
			Name:        p.Name,
			Type:        p.Type,
			Description: p.Description,
			Default:     p.Default,
		})
	}
	return ArtifactDetailOutput{
		ArtifactSummaryOutput: toArtifactSummaryOutput(a.ArtifactSummary),
		Parameters:            params,
	}
}

// GetArtifactDetailsInput names the artifact to retrieve. Name is
// validated with internal/validation.ArtifactName before any
// Velociraptor call, so malformed input never reaches the network.
type GetArtifactDetailsInput struct {
	Name string `json:"name" jsonschema:"Velociraptor artifact name, e.g. Windows.System.Pslist"`
}

// GetArtifactDetailsOutput reports the artifact's parameter schema plus,
// honestly, mode and any failure message; Artifact is nil whenever no
// data was returned (mock mode, policy block, or a failed real call).
type GetArtifactDetailsOutput struct {
	Mode     string                `json:"mode"`
	Artifact *ArtifactDetailOutput `json:"artifact,omitempty"`
	Message  string                `json:"message,omitempty"`
}

func newGetArtifactDetailsHandler(deps Deps) mcp.ToolHandlerFor[GetArtifactDetailsInput, GetArtifactDetailsOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in GetArtifactDetailsInput) (*mcp.CallToolResult, GetArtifactDetailsOutput, error) {
		if err := validation.ArtifactName(in.Name); err != nil {
			recordAudit(deps, audit.Event{
				Tool:     "velo_get_artifact_details",
				Outcome:  audit.OutcomeBlocked,
				Artifact: in.Name,
				Reason:   "invalid artifact name syntax",
			})
			return nil, GetArtifactDetailsOutput{}, fmt.Errorf("invalid artifact name %q", in.Name)
		}

		// Visibility is allowlist-scoped by default, same as
		// velo_list_artifact_names; see ArtifactReader's doc comment.
		if deps.Policy != nil && !deps.Policy.AllowListAllArtifacts() && !deps.Policy.ArtifactAllowed(in.Name) {
			recordAudit(deps, audit.Event{
				Tool:     "velo_get_artifact_details",
				Outcome:  audit.OutcomeBlocked,
				Artifact: in.Name,
				Reason:   "artifact not in allowlist",
			})
			return nil, GetArtifactDetailsOutput{}, fmt.Errorf("artifact %q is not in the configured allowlist", in.Name)
		}

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{
				Tool:     "velo_get_artifact_details",
				Outcome:  audit.OutcomeSuccess,
				Artifact: in.Name,
				Reason:   "mock mode, no Velociraptor call made",
			})
			return nil, GetArtifactDetailsOutput{
				Mode:    VelociraptorModeMock,
				Message: "MCP server is running in mock mode (velociraptor.read_api_config_path is not configured); no Velociraptor call was made",
			}, nil
		}

		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{
				Tool:     "velo_get_artifact_details",
				Outcome:  audit.OutcomeError,
				Artifact: in.Name,
				Reason:   "VelociraptorReadMode is real but ReadClient is nil",
			})
			return nil, GetArtifactDetailsOutput{
				Mode:    VelociraptorModeReal,
				Message: "real mode is configured but no Velociraptor client is available",
			}, nil
		}

		detail, err := deps.ReadClient.GetArtifactDetails(ctx, in.Name)
		if err != nil {
			recordAudit(deps, audit.Event{
				Tool:     "velo_get_artifact_details",
				Outcome:  audit.OutcomeError,
				Artifact: in.Name,
				Reason:   err.Error(),
			})
			return nil, GetArtifactDetailsOutput{
				Mode:    VelociraptorModeReal,
				Message: err.Error(),
			}, nil
		}

		out := toArtifactDetailOutput(detail)
		recordAudit(deps, audit.Event{
			Tool:     "velo_get_artifact_details",
			Outcome:  audit.OutcomeSuccess,
			Artifact: in.Name,
		})

		return nil, GetArtifactDetailsOutput{Mode: VelociraptorModeReal, Artifact: &out}, nil
	}
}
