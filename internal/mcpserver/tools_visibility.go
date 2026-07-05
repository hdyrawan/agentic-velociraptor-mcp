package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
)

// VisibilityTools are read-only, always-on tools requiring no approval:
// health, client discovery, and artifact catalog browsing.
//
// Only velo_health_check is registered with the MCP server as of
// v0.1.0-alpha.2 (see registerVisibilityTools): a real Velociraptor
// health check when Deps.VelociraptorReadMode is "real", or the
// original static mock when it is "mock". The other four remain
// metadata-only until v0.1.0, per PROJECT_PLAN.md.
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

// registerVisibilityTools registers the subset of VisibilityTools that
// are actually safe and implemented for this release: velo_health_check
// only.
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
