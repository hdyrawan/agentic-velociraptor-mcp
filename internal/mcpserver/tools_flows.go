package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
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
// plus the one approval-gated upload download. velo_list_flows,
// velo_get_flow_status, and velo_get_flow_results (v0.5.0) and
// velo_list_flow_uploads, velo_get_flow_upload_metadata, and
// velo_download_flow_upload_with_approval (v0.4.0) are all implemented
// and registered; see registerFlowTools.
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
		Description:      "Download bytes of one flow upload, bounded by max_upload_bytes, and save to a local operator-configured directory. Requires case_id, reason, requester, and a pre-approved approval_reference; treated as evidence disclosure, not a read.",
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

// registerFlowTools registers all six FlowTools entries: the three
// v0.5.0 read-only flow/result tools, the two v0.4.0 read-only upload
// tools, and the one v0.4.0 approval-gated download tool.
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

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_list_flow_uploads",
		Description: FlowTools[3].Description,
		Annotations: readOnlyAnnotations("List Velociraptor flow uploads"),
	}, newListFlowUploadsHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_get_flow_upload_metadata",
		Description: FlowTools[4].Description,
		Annotations: readOnlyAnnotations("Get Velociraptor flow upload metadata"),
	}, newGetFlowUploadMetadataHandler(deps))

	mcp.AddTool(s, &mcp.Tool{
		Name:        "velo_download_flow_upload_with_approval",
		Description: FlowTools[5].Description,
		Annotations: writeAnnotations("Download Velociraptor flow upload (approval-gated)"),
	}, newDownloadFlowUploadHandler(deps))
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

// UploadSummaryOutput mirrors velociraptor.UploadSummary with explicit
// JSON tags for the MCP tool response schema.
type UploadSummaryOutput struct {
	Component string `json:"component,omitempty"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Hash      string `json:"hash,omitempty"`
}

func toUploadSummaryOutput(u velociraptor.UploadSummary) UploadSummaryOutput {
	return UploadSummaryOutput{Component: u.Component, Name: u.Name, SizeBytes: u.SizeBytes, Hash: u.Hash}
}

// ListFlowUploadsInput names the client/flow to list uploads for.
type ListFlowUploadsInput struct {
	ClientID string `json:"client_id" jsonschema:"Velociraptor client ID, e.g. C.1234abcd5678ef90"`
	FlowID   string `json:"flow_id" jsonschema:"Velociraptor flow ID, e.g. F.BN2HJC4N4T6KG"`
}

// ListFlowUploadsOutput reports the upload list plus, honestly, mode and
// any failure message, following the same mock/real convention as the
// v0.1.0 visibility tools.
type ListFlowUploadsOutput struct {
	response.Result
	Mode     string                `json:"mode"`
	Uploads  []UploadSummaryOutput `json:"uploads"`
	ClientID string                `json:"client_id,omitempty"`
	FlowID   string                `json:"flow_id,omitempty"`
}

func newListFlowUploadsHandler(deps Deps) mcp.ToolHandlerFor[ListFlowUploadsInput, ListFlowUploadsOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ListFlowUploadsInput) (*mcp.CallToolResult, ListFlowUploadsOutput, error) {
		const tool = "velo_list_flow_uploads"

		if err := validation.ClientID(in.ClientID); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "invalid client id syntax"})
			return nil, ListFlowUploadsOutput{}, fmt.Errorf("invalid client id %q", in.ClientID)
		}
		if err := validation.FlowID(in.FlowID); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "invalid flow id syntax"})
			return nil, ListFlowUploadsOutput{}, fmt.Errorf("invalid flow id %q", in.FlowID)
		}

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "mock mode, no Velociraptor call made"})
			return nil, ListFlowUploadsOutput{
				Result:   response.Success("MCP server is running in mock mode (velociraptor.read_api_config_path is not configured); no Velociraptor call was made"),
				Mode:     VelociraptorModeMock,
				Uploads:  []UploadSummaryOutput{},
				ClientID: in.ClientID,
				FlowID:   in.FlowID,
			}, nil
		}
		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "VelociraptorReadMode is real but ReadClient is nil"})
			return nil, ListFlowUploadsOutput{
				Result:   response.Error("real mode is configured but no Velociraptor client is available"),
				Mode:     VelociraptorModeReal,
				Uploads:  []UploadSummaryOutput{},
				ClientID: in.ClientID,
				FlowID:   in.FlowID,
			}, nil
		}

		uploads, err := deps.ReadClient.ListFlowUploads(ctx, in.ClientID, in.FlowID)
		if err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, Reason: err.Error()})
			return nil, ListFlowUploadsOutput{
				Result:   response.Error(err.Error()),
				Mode:     VelociraptorModeReal,
				Uploads:  []UploadSummaryOutput{},
				ClientID: in.ClientID,
				FlowID:   in.FlowID,
			}, nil
		}

		out := make([]UploadSummaryOutput, 0, len(uploads))
		for _, u := range uploads {
			out = append(out, toUploadSummaryOutput(u))
		}
		recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, FlowID: in.FlowID, RowCount: len(out)})
		return nil, ListFlowUploadsOutput{
			Result:   response.Result{Status: response.StatusForCount(len(out))},
			Mode:     VelociraptorModeReal,
			Uploads:  out,
			ClientID: in.ClientID,
			FlowID:   in.FlowID,
		}, nil
	}
}

// GetFlowUploadMetadataInput names the client/flow/upload to fetch
// metadata for.
type GetFlowUploadMetadataInput struct {
	ClientID   string `json:"client_id" jsonschema:"Velociraptor client ID, e.g. C.1234abcd5678ef90"`
	FlowID     string `json:"flow_id" jsonschema:"Velociraptor flow ID, e.g. F.BN2HJC4N4T6KG"`
	UploadName string `json:"upload_name" jsonschema:"upload component/name as returned by velo_list_flow_uploads"`
}

// GetFlowUploadMetadataOutput reports upload metadata plus, honestly,
// mode and any failure message. Status distinguishes a genuine "no such
// upload" lookup (response.StatusNotFound, via
// velociraptor.ErrUploadNotFound) from any other connectivity/RPC
// failure (response.StatusError).
type GetFlowUploadMetadataOutput struct {
	response.Result
	Mode   string               `json:"mode"`
	Upload *UploadSummaryOutput `json:"upload,omitempty"`
}

func newGetFlowUploadMetadataHandler(deps Deps) mcp.ToolHandlerFor[GetFlowUploadMetadataInput, GetFlowUploadMetadataOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in GetFlowUploadMetadataInput) (*mcp.CallToolResult, GetFlowUploadMetadataOutput, error) {
		const tool = "velo_get_flow_upload_metadata"

		if err := validation.ClientID(in.ClientID); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "invalid client id syntax"})
			return nil, GetFlowUploadMetadataOutput{}, fmt.Errorf("invalid client id %q", in.ClientID)
		}
		if err := validation.FlowID(in.FlowID); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "invalid flow id syntax"})
			return nil, GetFlowUploadMetadataOutput{}, fmt.Errorf("invalid flow id %q", in.FlowID)
		}
		if err := validation.UploadName(in.UploadName); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "invalid upload name"})
			return nil, GetFlowUploadMetadataOutput{}, err
		}

		if deps.VelociraptorReadMode != VelociraptorModeReal {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "mock mode, no Velociraptor call made"})
			return nil, GetFlowUploadMetadataOutput{
				Result: response.Success("MCP server is running in mock mode (velociraptor.read_api_config_path is not configured); no Velociraptor call was made"),
				Mode:   VelociraptorModeMock,
			}, nil
		}
		if deps.ReadClient == nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, Reason: "VelociraptorReadMode is real but ReadClient is nil"})
			return nil, GetFlowUploadMetadataOutput{
				Result: response.Error("real mode is configured but no Velociraptor client is available"),
				Mode:   VelociraptorModeReal,
			}, nil
		}

		upload, err := deps.ReadClient.GetFlowUploadMetadata(ctx, in.ClientID, in.FlowID, in.UploadName)
		if err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, Reason: err.Error()})
			result := response.Error(err.Error())
			if errors.Is(err, velociraptor.ErrUploadNotFound) {
				result = response.NotFound(err.Error())
			}
			return nil, GetFlowUploadMetadataOutput{Result: result, Mode: VelociraptorModeReal}, nil
		}

		out := toUploadSummaryOutput(upload)
		recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, FlowID: in.FlowID})
		return nil, GetFlowUploadMetadataOutput{
			Result: response.Result{Status: response.StatusSuccess},
			Mode:   VelociraptorModeReal,
			Upload: &out,
		}, nil
	}
}

// DownloadFlowUploadInput is velo_download_flow_upload_with_approval's
// argument shape.
type DownloadFlowUploadInput struct {
	ClientID          string `json:"client_id" jsonschema:"Velociraptor client ID, e.g. C.1234abcd5678ef90"`
	FlowID            string `json:"flow_id" jsonschema:"Velociraptor flow ID, e.g. F.BN2HJC4N4T6KG"`
	UploadName        string `json:"upload_name" jsonschema:"upload component/name as returned by velo_list_flow_uploads"`
	CaseID            string `json:"case_id" jsonschema:"investigation/case identifier this download is tied to"`
	Reason            string `json:"reason" jsonschema:"justification for why this download is needed"`
	Requester         string `json:"requester" jsonschema:"identity of whoever is asking for this download"`
	ApprovalReference string `json:"approval_reference" jsonschema:"reference to a pre-approved request created via the approve CLI subcommand"`
}

// DownloadFlowUploadOutput reports the outcome of a download attempt.
// Raw evidence bytes are never returned inline: on success, the bytes
// are written to a local file under
// config.VelociraptorConfig.DownloadDir and only that path, its size,
// and a SHA-256 checksum are reported.
type DownloadFlowUploadOutput struct {
	response.Result
	ClientID  string `json:"client_id,omitempty"`
	FlowID    string `json:"flow_id,omitempty"`
	LocalPath string `json:"local_path,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// downloadFilename derives a local filename from already-validated
// clientID/flowID (strict charset, no path separators) plus a random
// suffix. It never incorporates the caller-supplied uploadName, which
// may contain arbitrary VFS-path-shaped text, so a crafted upload_name
// can never influence where the file is written.
func downloadFilename(clientID, flowID string) (string, error) {
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("generate download filename: %w", err)
	}
	return fmt.Sprintf("%s_%s_%d_%s.bin", clientID, flowID, time.Now().UnixNano(), hex.EncodeToString(suffix)), nil
}

func newDownloadFlowUploadHandler(deps Deps) mcp.ToolHandlerFor[DownloadFlowUploadInput, DownloadFlowUploadOutput] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in DownloadFlowUploadInput) (*mcp.CallToolResult, DownloadFlowUploadOutput, error) {
		const tool = "velo_download_flow_upload_with_approval"

		if enabled, reason := writePilotEnabled(deps); !enabled {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, Reason: reason})
			return nil, DownloadFlowUploadOutput{}, errors.New(reason)
		}
		if deps.Config == nil || deps.Config.Velociraptor.DownloadDir == "" {
			const reason = "the controlled write pilot is disabled: velociraptor.download_dir is not configured"
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, Reason: reason})
			return nil, DownloadFlowUploadOutput{}, errors.New(reason)
		}

		if err := validateApprovalFields(in.ClientID, in.CaseID, in.Reason, in.Requester, in.ApprovalReference); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, Reason: err.Error()})
			return nil, DownloadFlowUploadOutput{}, err
		}
		if err := validation.FlowID(in.FlowID); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, Reason: "invalid flow id syntax"})
			return nil, DownloadFlowUploadOutput{}, fmt.Errorf("invalid flow id %q", in.FlowID)
		}
		if err := validation.UploadName(in.UploadName); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeBlocked, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, Reason: "invalid upload name"})
			return nil, DownloadFlowUploadOutput{}, err
		}

		candidate := approval.Request{
			Operation:  approval.OperationDownloadFlowUpload,
			CaseID:     in.CaseID,
			ClientID:   in.ClientID,
			FlowID:     in.FlowID,
			UploadName: in.UploadName,
		}
		result, outcome, ok := verifyApproval(ctx, deps, in.ApprovalReference, candidate)
		if !ok {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: outcome, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference, Reason: result.Message})
			return nil, DownloadFlowUploadOutput{Result: result, ClientID: in.ClientID, FlowID: in.FlowID}, nil
		}
		if result := backendOperationReady(deps.WriteClient, velociraptor.BackendOpDownloadFlowUpload); result.Status != "" {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, Reason: result.Message})
			return nil, DownloadFlowUploadOutput{Result: result, ClientID: in.ClientID, FlowID: in.FlowID}, nil
		}
		if result, ok := gateAuditForWrite(deps, audit.Event{Tool: tool, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference}); !ok {
			return nil, DownloadFlowUploadOutput{Result: result, ClientID: in.ClientID, FlowID: in.FlowID}, nil
		}

		// Pre-flight: ensure DownloadDir exists and is writable
		// before burning the single-use approval. This avoids
		// wasting an approval on a configuration error (e.g.
		// read-only filesystem, missing parent dir, permission
		// denied) that would only surface after the approval
		// has been consumed.
		if err := os.MkdirAll(deps.Config.Velociraptor.DownloadDir, 0o700); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, ApprovalID: in.ApprovalReference, Reason: "download_dir not writable: " + err.Error()})
			return nil, DownloadFlowUploadOutput{Result: response.Error("download_dir is not writable: " + err.Error()), ClientID: in.ClientID, FlowID: in.FlowID}, nil
		}

		result, outcome, ok = consumeApproval(ctx, deps, in.ApprovalReference)
		if !ok {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: outcome, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference, Reason: result.Message})
			return nil, DownloadFlowUploadOutput{Result: result, ClientID: in.ClientID, FlowID: in.FlowID}, nil
		}

		maxBytes := deps.Config.Velociraptor.MaxUploadBytes
		data, err := deps.WriteClient.DownloadFlowUpload(ctx, in.ClientID, in.FlowID, in.UploadName, maxBytes)
		if err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, ApprovalID: in.ApprovalReference, Reason: err.Error()})
			result := response.Error(err.Error())
			if errors.Is(err, velociraptor.ErrUploadNotFound) {
				result = response.NotFound(err.Error())
			}
			return nil, DownloadFlowUploadOutput{Result: result, ClientID: in.ClientID, FlowID: in.FlowID}, nil
		}

		filename, err := downloadFilename(in.ClientID, in.FlowID)
		if err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, ApprovalID: in.ApprovalReference, Reason: err.Error()})
			return nil, DownloadFlowUploadOutput{Result: response.Error(err.Error()), ClientID: in.ClientID, FlowID: in.FlowID}, nil
		}
		localPath := filepath.Join(deps.Config.Velociraptor.DownloadDir, filename)
		if err := os.WriteFile(localPath, data, 0o600); err != nil {
			recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeError, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, ApprovalID: in.ApprovalReference, Reason: err.Error()})
			return nil, DownloadFlowUploadOutput{Result: response.Error(err.Error()), ClientID: in.ClientID, FlowID: in.FlowID}, nil
		}

		sum := sha256.Sum256(data)
		recordAudit(deps, audit.Event{Tool: tool, Outcome: audit.OutcomeSuccess, ClientID: in.ClientID, FlowID: in.FlowID, CaseID: in.CaseID, RequestReason: in.Reason, ApprovalID: in.ApprovalReference, ByteCount: int64(len(data))})
		return nil, DownloadFlowUploadOutput{
			Result:    response.Result{Status: response.StatusSuccess},
			ClientID:  in.ClientID,
			FlowID:    in.FlowID,
			LocalPath: localPath,
			SizeBytes: int64(len(data)),
			SHA256:    hex.EncodeToString(sum[:]),
			Truncated: maxBytes > 0 && int64(len(data)) >= maxBytes,
		}, nil
	}
}
