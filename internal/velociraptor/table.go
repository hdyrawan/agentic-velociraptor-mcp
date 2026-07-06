package velociraptor

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor/veloapi"
)

// decodeTableRows converts a Velociraptor GetTableResponse into generic
// rows keyed by column name. Velociraptor's GetTable/GetClientFlows/
// GetHuntResults RPCs all return this same shape: a fixed column-name
// list plus one JSON-encoded array per row (Row.json), positionally
// matching the columns. This is the only place this project parses that
// row wire format, and it only ever treats the decoded values as opaque
// JSON data (strings/numbers/bools/nested arrays) — never as VQL or
// executable text.
func decodeTableRows(resp *veloapi.GetTableResponse) ([]map[string]any, error) {
	if resp == nil {
		return nil, nil
	}
	columns := resp.GetColumns()
	rawRows := resp.GetRows()
	out := make([]map[string]any, 0, len(rawRows))
	for _, r := range rawRows {
		var values []any
		if err := json.Unmarshal([]byte(r.GetJson()), &values); err != nil {
			return nil, fmt.Errorf("velociraptor: decode table row: %w", err)
		}
		row := make(map[string]any, len(columns))
		for i, col := range columns {
			if i < len(values) {
				row[col] = values[i]
			} else {
				row[col] = nil
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// rowStr reads the first present key in keys from row as a string. This
// tolerant multi-key lookup exists because some Velociraptor result-set
// tables (notably the "uploads" table backing ListFlowUploads/
// GetFlowUploadMetadata) are serialized generically from a Go struct
// rather than declared with fixed column names the way GetClientFlows/
// GetHuntTable are, so the exact key casing has not been confirmed
// against a live server; see docs/lab-validation-plan.md.
func rowStr(row map[string]any, keys ...string) string {
	for _, k := range keys {
		v, ok := row[k]
		if !ok || v == nil {
			continue
		}
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// rowInt64 reads the first present key in keys from row as an int64.
// JSON numbers decode into float64 via encoding/json's generic
// unmarshaling, so that is the only numeric representation handled
// here.
func rowInt64(row map[string]any, keys ...string) int64 {
	for _, k := range keys {
		v, ok := row[k]
		if !ok || v == nil {
			continue
		}
		if f, ok := v.(float64); ok {
			return int64(f)
		}
	}
	return 0
}

// rowStringSlice reads the first present key in keys from row as a
// string slice (a JSON array of strings decodes into []any of string
// values via encoding/json's generic unmarshaling).
func rowStringSlice(row map[string]any, keys ...string) []string {
	for _, k := range keys {
		v, ok := row[k]
		if !ok || v == nil {
			continue
		}
		items, ok := v.([]any)
		if !ok {
			continue
		}
		out := make([]string, 0, len(items))
		for _, item := range items {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// parseOffsetCursor parses this project's "offset:N" pagination cursor
// convention (also used by internal/mcpserver's nextOffsetCursor),
// returning 0 for an empty, malformed, or negative cursor rather than
// erroring — an invalid cursor degrades to "start from the beginning"
// rather than failing the call.
func parseOffsetCursor(cursor string) int {
	if !strings.HasPrefix(cursor, "offset:") {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimPrefix(cursor, "offset:"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// offsetCursor formats this project's "offset:N" pagination cursor.
func offsetCursor(offset int) string {
	return fmt.Sprintf("offset:%d", offset)
}
