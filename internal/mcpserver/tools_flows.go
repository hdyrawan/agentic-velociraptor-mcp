package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/validation"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

const (
	defaultToolMaxRows        = 100
	defaultToolMaxResultBytes = 1048576
)

// FlowTools cover read access to collection flow status/results/uploads,
// plus the one approval-gated upload download. v0.5.0 registers only the
// first three read-only flow/result tools. Upload listing/metadata and
// download remain unregistered: downloads disclose evidence bytes and
// belong to a future approval-gated collection/download path.
var FlowTools = []ToolSpec{
	{
		Name:        "velo_list_flows",
		Description: "List collection flows for a client. Read-only; validates client_id and applies max_rows plus cursor pagination.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_flow_status",
		Description: "Get the state of one flow (running/finished/error/cancelled). Read-only; validates client_id and flow_id.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_flow_results",
		Description: "Get result rows for one flow, bounded by max_rows/max_result_bytes; response states explicitly if truncated.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_list_flow_uploads",
		Description: "List uploaded files attached to a flow's results, without content.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_flow_upload_metadata",
		Description: "Get size/hash metadata for one flow upload, without content.",
		ReadOnly:    true,
	},
	{
		Name:             "velo_download_flow_upload_with_approval",
		Description:      "Download bytes of one flow upload, bounded by max_upload_bytes. Requires prior approval; treated as evidence disclosure, not a read.",
		RequiresApproval: true,
	},
}

type ListFlowsInput struct {
	ClientID string `json:"client_id" jsonschema:"Velociraptor client ID, e.g. C.1234abcd5678ef90"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum number of flows to return; server-side max_rows ceiling applies"`
	Cursor   string `json:"cursor,omitempty" jsonschema:"opaque pagination cursor returned by a previous call"`
}

type FlowSummaryOutput struct {
	FlowID    string `json:"flow_id"`
	ClientID  string `json:"client_id"`
	Artifact  string `json:"artifact,omitempty"`
	State     string `json:"state,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type ListFlowsOutput struct {
	response.Result
	Mode       string              `json:"mode"`
	ClientID   string              `json:"client_id"`
	Flows      []FlowSummaryOutput `json:"flows"`
	NextCursor string              `json:"next_cursor,omitempty"`
	Truncated  bool                `json:"truncated"`
}

type GetFlowStatusInput struct {
	ClientID string `json:"client_id" jsonschema:"Velociraptor client ID, e.g. C.1234abcd5678ef90"`
	FlowID   string `json:"flow_id" jsonschema:"Velociraptor flow ID, e.g. F.BN2HJC4N4T6KG"`
}

type GetFlowStatusOutput struct {
	response.Result
	Mode string             `json:"mode"`
	Flow *FlowSummaryOutput `json:"flow,omitempty"`
}

type GetFlowResultsInput struct {
	ClientID string `json:"client_id" jsonschema:"Velociraptor client ID, e.g. C.1234abcd5678ef90"`
	FlowID   string `json:"flow_id" jsonschema:"Velociraptor flow ID, e.g. F.BN2HJC4N4T6KG"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum result rows to return; server-side max_rows ceiling applies"`
	Cursor   string `json:"cursor,omitempty" jsonschema:"opaque pagination cursor returned by a previous call"`
}

type GetFlowResultsOutput struct {
	response.Result
	Mode         string           `json:"mode"`
	ClientID     string           `json:"client_id"`
	FlowID       string           `json:"flow_id"`
	Rows         []map[string]any `json:"rows"`
	ReturnedRows int              `json:"returned_rows"`
	TotalRows    int              `json:"total_rows,omitempty"`
	ByteCount    int64            `json:"byte_count"`
	NextCursor   string           `json:"next_cursor,omitempty"`
	Truncated    bool             `json:"truncated"`
}

func registerFlowTools(s *mcp.Server, deps Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_list_flows",
		Description: FlowTools[0].Description,
		Annotations: readOnlyAnnotations("List Velociraptor flows"),
	}, newListFlowsHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_get_flow_status",
		Description: FlowTools[1].Description,
		Annotations: readOnlyAnnotations("Get Velociraptor flow status"),
	}, newGetFlowStatusHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_get_flow_results",
		Description: FlowTools[2].Description,
		Annotations: readOnlyAnnotations("Get Velociraptor flow results"),
	}, newGetFlowResultsHandler(deps))
}

func newListFlowsHandler(deps Deps) mcp.ToolHandlerFor[ListFlowsInput, ListFlowsOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ListFlowsInput) (*mcp.CallToolResult, ListFlowsOutput, error) {
		if err := validateFlowReadInput(in.ClientID, ""); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_list_flows", Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, Reason: err.Error()})
			return nil, ListFlowsOutput{}, err
		}
		limit := boundToolLimit(in.Limit, configuredMaxRows(deps))

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{Tool: "velo_list_flows", Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, Reason: "mock mode, no Velociraptor call made"})
			return nil, ListFlowsOutput{Result: response.Success("MCP server is running in mock mode (velociraptor.read_api_config_path is not configured); no Velociraptor call was made"), Mode: VelociraptorModeMock, ClientID: in.ClientID, Flows: []FlowSummaryOutput{}}, nil
		}
		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{Tool: "velo_list_flows", Outcome: audit.OutcomeError, ClientID: in.ClientID, Reason: "VelociraptorReadMode is real but ReadClient is nil"})
			return nil, ListFlowsOutput{Result: response.Error("real mode is configured but no Velociraptor client is available"), Mode: VelociraptorModeReal, ClientID: in.ClientID, Flows: []FlowSummaryOutput{}}, nil
		}

		flows, err := deps.ReadClient.ListFlows(ctx, in.ClientID, limit, in.Cursor)
		if err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_list_flows", Outcome: audit.OutcomeError, ClientID: in.ClientID, Reason: err.Error()})
			result := response.Error(err.Error())
			if errors.Is(err, velociraptor.ErrClientNotFound) {
				result = response.NotFound(err.Error())
			}
			return nil, ListFlowsOutput{Result: result, Mode: VelociraptorModeReal, ClientID: in.ClientID, Flows: []FlowSummaryOutput{}}, nil
		}

		truncated := len(flows) > limit
		if truncated {
			flows = flows[:limit]
		}
		out := make([]FlowSummaryOutput, 0, len(flows))
		for _, f := range flows {
			out = append(out, toFlowSummaryOutput(f))
		}

		recordAudit(deps, audit.Event{Tool: "velo_list_flows", Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, RowCount: len(out)})
		return nil, ListFlowsOutput{Result: response.Result{Status: response.StatusForCount(len(out))}, Mode: VelociraptorModeReal, ClientID: in.ClientID, Flows: out, NextCursor: nextOffsetCursor(in.Cursor, len(out), truncated), Truncated: truncated}, nil
	}
}

func newGetFlowStatusHandler(deps Deps) mcp.ToolHandlerFor[GetFlowStatusInput, GetFlowStatusOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in GetFlowStatusInput) (*mcp.CallToolResult, GetFlowStatusOutput, error) {
		if err := validateFlowReadInput(in.ClientID, in.FlowID); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_get_flow_status", Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, Reason: err.Error()})
			return nil, GetFlowStatusOutput{}, err
		}

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{Tool: "velo_get_flow_status", Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "mock mode, no Velociraptor call made"})
			return nil, GetFlowStatusOutput{Result: response.Success("MCP server is running in mock mode (velociraptor.read_api_config_path is not configured); no Velociraptor call was made"), Mode: VelociraptorModeMock}, nil
		}
		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{Tool: "velo_get_flow_status", Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "VelociraptorReadMode is real but ReadClient is nil"})
			return nil, GetFlowStatusOutput{Result: response.Error("real mode is configured but no Velociraptor client is available"), Mode: VelociraptorModeReal}, nil
		}

		flow, err := deps.ReadClient.GetFlowStatus(ctx, in.ClientID, in.FlowID)
		if err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_get_flow_status", Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, Reason: err.Error()})
			result := response.Error(err.Error())
			if errors.Is(err, velociraptor.ErrFlowNotFound) || errors.Is(err, velociraptor.ErrClientNotFound) {
				result = response.NotFound(err.Error())
			}
			return nil, GetFlowStatusOutput{Result: result, Mode: VelociraptorModeReal}, nil
		}

		out := toFlowSummaryOutput(flow)
		recordAudit(deps, audit.Event{Tool: "velo_get_flow_status", Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, FlowID: in.FlowID})
		return nil, GetFlowStatusOutput{Result: response.Result{Status: response.StatusSuccess}, Mode: VelociraptorModeReal, Flow: &out}, nil
	}
}

func newGetFlowResultsHandler(deps Deps) mcp.ToolHandlerFor[GetFlowResultsInput, GetFlowResultsOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in GetFlowResultsInput) (*mcp.CallToolResult, GetFlowResultsOutput, error) {
		if err := validateFlowReadInput(in.ClientID, in.FlowID); err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_get_flow_results", Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, Reason: err.Error()})
			return nil, GetFlowResultsOutput{}, err
		}
		limit := boundToolLimit(in.Limit, configuredMaxRows(deps))
		maxBytes := configuredMaxResultBytes(deps)

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{Tool: "velo_get_flow_results", Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "mock mode, no Velociraptor call made"})
			return nil, GetFlowResultsOutput{Result: response.Success("MCP server is running in mock mode (velociraptor.read_api_config_path is not configured); no Velociraptor call was made"), Mode: VelociraptorModeMock, ClientID: in.ClientID, FlowID: in.FlowID, Rows: []map[string]any{}}, nil
		}
		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{Tool: "velo_get_flow_results", Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "VelociraptorReadMode is real but ReadClient is nil"})
			return nil, GetFlowResultsOutput{Result: response.Error("real mode is configured but no Velociraptor client is available"), Mode: VelociraptorModeReal, ClientID: in.ClientID, FlowID: in.FlowID, Rows: []map[string]any{}}, nil
		}

		page, err := deps.ReadClient.GetFlowResults(ctx, in.ClientID, in.FlowID, limit, maxBytes, in.Cursor)
		if err != nil {
			recordAudit(deps, audit.Event{Tool: "velo_get_flow_results", Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, Reason: err.Error()})
			result := response.Error(err.Error())
			if errors.Is(err, velociraptor.ErrFlowNotFound) || errors.Is(err, velociraptor.ErrClientNotFound) {
				result = response.NotFound(err.Error())
			}
			return nil, GetFlowResultsOutput{Result: result, Mode: VelociraptorModeReal, ClientID: in.ClientID, FlowID: in.FlowID, Rows: []map[string]any{}}, nil
		}

		rows, byteCount, truncatedByHandler := boundRowsByLimitAndBytes(page.Rows, limit, maxBytes)
		truncated := page.Truncated || truncatedByHandler
		nextCursor := page.NextCursor
		if nextCursor == "" {
			nextCursor = nextOffsetCursor(in.Cursor, len(rows), truncated)
		}
		totalRows := page.TotalRows
		if totalRows == 0 && len(page.Rows) > 0 {
			totalRows = len(page.Rows)
		}
		result := response.Result{Status: response.StatusForCount(len(rows))}
		if len(rows) == 0 && truncatedByHandler && len(page.Rows) > 0 {
			result = response.Error("flow result rows exceed configured max_result_bytes")
		}

		recordAudit(deps, audit.Event{Tool: "velo_get_flow_results", Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, FlowID: in.FlowID, RowCount: len(rows), ByteCount: byteCount})
		return nil, GetFlowResultsOutput{Result: result, Mode: VelociraptorModeReal, ClientID: in.ClientID, FlowID: in.FlowID, Rows: rows, ReturnedRows: len(rows), TotalRows: totalRows, ByteCount: byteCount, NextCursor: nextCursor, Truncated: truncated}, nil
	}
}

func toFlowSummaryOutput(f velociraptor.FlowSummary) FlowSummaryOutput {
	return FlowSummaryOutput{FlowID: f.FlowID, ClientID: f.ClientID, Artifact: f.Artifact, State: string(f.State), CreatedAt: f.CreatedAt}
}

func validateFlowReadInput(clientID, flowID string) error {
	if err := validation.ClientID(clientID); err != nil {
		return fmt.Errorf("invalid client id %q", clientID)
	}
	if flowID != "" {
		if err := validation.FlowID(flowID); err != nil {
			return fmt.Errorf("invalid flow id %q", flowID)
		}
	}
	return nil
}

func configuredMaxRows(deps Deps) int {
	if deps.Config != nil && deps.Config.Velociraptor.MaxRows > 0 {
		return deps.Config.Velociraptor.MaxRows
	}
	return defaultToolMaxRows
}

func configuredMaxResultBytes(deps Deps) int64 {
	if deps.Config != nil && deps.Config.Velociraptor.MaxResultBytes > 0 {
		return deps.Config.Velociraptor.MaxResultBytes
	}
	return defaultToolMaxResultBytes
}

func boundToolLimit(requested, ceiling int) int {
	if ceiling <= 0 {
		ceiling = defaultToolMaxRows
	}
	if requested <= 0 || requested > ceiling {
		return ceiling
	}
	return requested
}

func boundRowsByLimitAndBytes(rows []map[string]any, limit int, maxBytes int64) ([]map[string]any, int64, bool) {
	if limit <= 0 {
		limit = defaultToolMaxRows
	}
	if maxBytes <= 0 {
		maxBytes = defaultToolMaxResultBytes
	}
	out := make([]map[string]any, 0, minInt(len(rows), limit))
	var total int64
	truncated := len(rows) > limit
	for _, row := range rows {
		if len(out) >= limit {
			truncated = true
			break
		}
		b, err := json.Marshal(row)
		if err != nil {
			b = []byte(fmt.Sprint(row))
		}
		rowBytes := int64(len(b))
		if total+rowBytes > maxBytes {
			truncated = true
			break
		}
		out = append(out, row)
		total += rowBytes
	}
	return out, total, truncated
}

func nextOffsetCursor(cursor string, returned int, truncated bool) string {
	if !truncated || returned == 0 {
		return ""
	}
	offset := 0
	if strings.HasPrefix(cursor, "offset:") {
		parsed, err := strconv.Atoi(strings.TrimPrefix(cursor, "offset:"))
		if err == nil && parsed > 0 {
			offset = parsed
		}
	}
	return fmt.Sprintf("offset:%d", offset+returned)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
