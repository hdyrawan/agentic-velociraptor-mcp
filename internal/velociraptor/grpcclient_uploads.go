package velociraptor

import (
	"context"
	"fmt"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor/veloapi"
)

// defaultMaxUploadBytes bounds DownloadFlowUpload when called with a
// non-positive maxBytes, mirroring defaultMaxRows's fail-closed default
// for result lists: an unconfigured limit must never mean "unbounded".
const defaultMaxUploadBytes = 10 * 1024 * 1024

// vfsChunkSize is the per-RPC read size DownloadFlowUpload requests from
// VFSGetBuffer, matching upstream Velociraptor's own download handler
// buffer size (api.BUFSIZE).
const vfsChunkSize = 1 << 20

// uploadRow is the internal, richer decoding of one row of a flow's
// "uploads" GetTable(type="uploads") result: unlike the public
// UploadSummary this project's tools return, it also carries the
// file-store Components VFSGetBuffer needs to fetch content.
//
// The exact column-name casing of this table has not been confirmed
// against a live Velociraptor server (it is a generically serialized
// result set, not one of the fixed-column tables like GetClientFlows;
// see decodeTableRows and rowStr's doc comment), so lookups below try
// multiple plausible spellings rather than assuming one.
type uploadRow struct {
	name       string
	components []string
	sizeBytes  int64
	kind       string
}

func uploadRowFromTableRow(row map[string]any) uploadRow {
	return uploadRow{
		name:       rowStr(row, "Name", "name"),
		components: rowStringSlice(row, "Components", "components"),
		sizeBytes:  rowInt64(row, "Size", "size"),
		kind:       rowStr(row, "Type", "type"),
	}
}

func (u uploadRow) toUploadSummary() UploadSummary {
	return UploadSummary{
		Component: u.kind,
		Name:      u.name,
		SizeBytes: u.sizeBytes,
	}
}

// listFlowUploadRows fetches and decodes the flow's uploads table.
func (c *grpcClient) listFlowUploadRows(ctx context.Context, clientID, flowID string) ([]uploadRow, error) {
	resp, err := c.tables.GetTable(ctx, &veloapi.GetTableRequest{
		ClientId: clientID,
		FlowId:   flowID,
		Type:     "uploads",
		Rows:     uint64(c.effectiveMaxRows()),
	})
	if err != nil {
		return nil, sanitizeTLSError(err)
	}
	rows, err := decodeTableRows(resp)
	if err != nil {
		return nil, err
	}
	out := make([]uploadRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, uploadRowFromTableRow(row))
	}
	return out, nil
}

// ListFlowUploads calls Velociraptor's GetTable RPC with type="uploads",
// the fixed literal that selects a flow's upload-metadata table (see
// api/tables/table.go's GetPathSpec upstream); never a caller-supplied
// table name.
func (c *grpcClient) ListFlowUploads(ctx context.Context, clientID, flowID string) ([]UploadSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	rows, err := c.listFlowUploadRows(ctx, clientID, flowID)
	if err != nil {
		return nil, fmt.Errorf("velociraptor: list flow uploads: %w", err)
	}
	out := make([]UploadSummary, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toUploadSummary())
	}
	return out, nil
}

// GetFlowUploadMetadata lists the flow's uploads (there is no
// per-name-filtered variant of the underlying GetTable RPC) and returns
// the one matching uploadName, or ErrUploadNotFound.
func (c *grpcClient) GetFlowUploadMetadata(ctx context.Context, clientID, flowID, uploadName string) (UploadSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	rows, err := c.listFlowUploadRows(ctx, clientID, flowID)
	if err != nil {
		return UploadSummary{}, fmt.Errorf("velociraptor: get flow upload metadata: %w", err)
	}
	for _, r := range rows {
		if r.name == uploadName {
			return r.toUploadSummary(), nil
		}
	}
	return UploadSummary{}, fmt.Errorf("velociraptor: upload %q: %w", uploadName, ErrUploadNotFound)
}

// DownloadFlowUpload fetches one upload's content by first locating its
// file-store Components (via the same uploads table
// ListFlowUploads/GetFlowUploadMetadata use), then reading it in
// vfsChunkSize chunks through Velociraptor's VFSGetBuffer RPC — the typed
// RPC upstream itself documents as being "for API clients to fetch file
// content" (as opposed to the GUI's HTTP-only download handlers, which
// this project never calls; see internal/velociraptor/veloapi/vfs.proto).
// The read stops at maxBytes even if the upload is larger, and Velociraptor
// (VFSGetBuffer's own EOF signal: a short or empty chunk) is trusted to
// end the read at the real file size.
func (c *grpcClient) DownloadFlowUpload(ctx context.Context, clientID, flowID, uploadName string, maxBytes int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	if maxBytes <= 0 {
		maxBytes = defaultMaxUploadBytes
	}

	rows, err := c.listFlowUploadRows(ctx, clientID, flowID)
	if err != nil {
		return nil, fmt.Errorf("velociraptor: download flow upload: %w", err)
	}
	var target *uploadRow
	for i := range rows {
		if rows[i].name == uploadName {
			target = &rows[i]
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("velociraptor: upload %q: %w", uploadName, ErrUploadNotFound)
	}
	if len(target.components) == 0 {
		return nil, fmt.Errorf("velociraptor: upload %q has no file-store location", uploadName)
	}

	toRead := maxBytes
	if target.sizeBytes > 0 && target.sizeBytes < toRead {
		toRead = target.sizeBytes
	}

	buf := make([]byte, 0, toRead)
	var offset uint64
	for int64(len(buf)) < toRead {
		length := vfsChunkSize
		if remaining := toRead - int64(len(buf)); int64(length) > remaining {
			length = int(remaining)
		}

		resp, err := c.vfs.VFSGetBuffer(ctx, &veloapi.VFSFileBuffer{
			ClientId:   clientID,
			Components: target.components,
			Offset:     offset,
			Length:     uint32(length),
		})
		if err != nil {
			return nil, fmt.Errorf("velociraptor: download flow upload: %w", sanitizeTLSError(err))
		}
		data := resp.GetData()
		if len(data) == 0 {
			break
		}
		buf = append(buf, data...)
		offset += uint64(len(data))
		if len(data) < length {
			break
		}
	}
	return buf, nil
}
