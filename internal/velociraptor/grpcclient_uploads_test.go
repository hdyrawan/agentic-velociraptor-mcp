package velociraptor

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor/veloapi"
)

// fakeVFSService implements vfsService for tests.
type fakeVFSService struct {
	vfsGetBuffer func(ctx context.Context, in *veloapi.VFSFileBuffer) (*veloapi.VFSFileBuffer, error)
}

func (f *fakeVFSService) VFSGetBuffer(ctx context.Context, in *veloapi.VFSFileBuffer, opts ...grpc.CallOption) (*veloapi.VFSFileBuffer, error) {
	return f.vfsGetBuffer(ctx, in)
}

func uploadsTableResponse(t *testing.T) *veloapi.GetTableResponse {
	t.Helper()
	return &veloapi.GetTableResponse{
		Columns: []string{"Name", "Components", "Size", "Type"},
		Rows: []*veloapi.Row{
			jsonRow(t, "memory.dmp", []string{"clients", "C.1234abcd5678ef90", "collection", "F.ABC123", "uploads", "memory.dmp"}, 5000, ""),
		},
	}
}

func TestGRPCClientListFlowUploadsSuccess(t *testing.T) {
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			if in.Type != "uploads" {
				t.Errorf("Type = %q, want uploads", in.Type)
			}
			if in.ClientId != "C.1234abcd5678ef90" || in.FlowId != "F.ABC123" {
				t.Errorf("request = %+v", in)
			}
			return uploadsTableResponse(t), nil
		},
	}
	c := &grpcClient{tables: tables, timeout: time.Second, maxRows: 100}

	uploads, err := c.ListFlowUploads(context.Background(), "C.1234abcd5678ef90", "F.ABC123")
	if err != nil {
		t.Fatalf("ListFlowUploads: %v", err)
	}
	if len(uploads) != 1 || uploads[0].Name != "memory.dmp" || uploads[0].SizeBytes != 5000 {
		t.Errorf("uploads = %+v", uploads)
	}
}

func TestGRPCClientListFlowUploadsError(t *testing.T) {
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			return nil, errors.New("connection refused")
		},
	}
	c := &grpcClient{tables: tables, timeout: time.Second, maxRows: 100}

	if _, err := c.ListFlowUploads(context.Background(), "C.1234abcd5678ef90", "F.ABC123"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGRPCClientGetFlowUploadMetadataSuccess(t *testing.T) {
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			return uploadsTableResponse(t), nil
		},
	}
	c := &grpcClient{tables: tables, timeout: time.Second, maxRows: 100}

	u, err := c.GetFlowUploadMetadata(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "memory.dmp")
	if err != nil {
		t.Fatalf("GetFlowUploadMetadata: %v", err)
	}
	if u.Name != "memory.dmp" || u.SizeBytes != 5000 {
		t.Errorf("upload = %+v", u)
	}
}

func TestGRPCClientGetFlowUploadMetadataNotFound(t *testing.T) {
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			return uploadsTableResponse(t), nil
		},
	}
	c := &grpcClient{tables: tables, timeout: time.Second, maxRows: 100}

	_, err := c.GetFlowUploadMetadata(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "does-not-exist.bin")
	if !errors.Is(err, ErrUploadNotFound) {
		t.Errorf("err = %v, want ErrUploadNotFound", err)
	}
}

func TestGRPCClientDownloadFlowUploadSuccess(t *testing.T) {
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			return &veloapi.GetTableResponse{
				Columns: []string{"Name", "Components", "Size"},
				Rows:    []*veloapi.Row{jsonRow(t, "small.bin", []string{"a", "b"}, 10)},
			}, nil
		},
	}
	var sawOffsets []uint64
	vfs := &fakeVFSService{
		vfsGetBuffer: func(ctx context.Context, in *veloapi.VFSFileBuffer) (*veloapi.VFSFileBuffer, error) {
			sawOffsets = append(sawOffsets, in.Offset)
			if in.ClientId != "C.1234abcd5678ef90" {
				t.Errorf("ClientId = %q", in.ClientId)
			}
			data := []byte("0123456789")[in.Offset:]
			return &veloapi.VFSFileBuffer{Data: data}, nil
		},
	}
	c := &grpcClient{tables: tables, vfs: vfs, timeout: time.Second, maxRows: 100}

	data, err := c.DownloadFlowUpload(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "small.bin", 1<<20)
	if err != nil {
		t.Fatalf("DownloadFlowUpload: %v", err)
	}
	if string(data) != "0123456789" {
		t.Errorf("data = %q, want 0123456789", data)
	}
	if len(sawOffsets) != 1 || sawOffsets[0] != 0 {
		t.Errorf("sawOffsets = %v, want a single chunk starting at 0 for a 10-byte file", sawOffsets)
	}
}

// TestGRPCClientDownloadFlowUploadRespectsMaxBytes is the download safety
// test: even though the upload is reported larger than maxBytes, the
// download must stop at maxBytes rather than reading the whole file.
func TestGRPCClientDownloadFlowUploadRespectsMaxBytes(t *testing.T) {
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			return &veloapi.GetTableResponse{
				Columns: []string{"Name", "Components", "Size"},
				Rows:    []*veloapi.Row{jsonRow(t, "huge.bin", []string{"a"}, 1_000_000)},
			}, nil
		},
	}
	var totalRequested int
	vfs := &fakeVFSService{
		vfsGetBuffer: func(ctx context.Context, in *veloapi.VFSFileBuffer) (*veloapi.VFSFileBuffer, error) {
			totalRequested += int(in.Length)
			// Always return a full chunk (as if the file really is huge),
			// so only maxBytes bounds the read, not a short/EOF response.
			return &veloapi.VFSFileBuffer{Data: make([]byte, in.Length)}, nil
		},
	}
	c := &grpcClient{tables: tables, vfs: vfs, timeout: time.Second, maxRows: 100}

	const maxBytes = 100
	data, err := c.DownloadFlowUpload(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "huge.bin", maxBytes)
	if err != nil {
		t.Fatalf("DownloadFlowUpload: %v", err)
	}
	if len(data) != maxBytes {
		t.Errorf("len(data) = %d, want exactly maxBytes=%d", len(data), maxBytes)
	}
	if totalRequested > maxBytes {
		t.Errorf("requested %d bytes total from VFSGetBuffer, want <= maxBytes=%d", totalRequested, maxBytes)
	}
}

func TestGRPCClientDownloadFlowUploadNotFound(t *testing.T) {
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			return &veloapi.GetTableResponse{Columns: []string{"Name", "Components", "Size"}}, nil
		},
	}
	c := &grpcClient{tables: tables, timeout: time.Second, maxRows: 100}

	_, err := c.DownloadFlowUpload(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "missing.bin", 1<<20)
	if !errors.Is(err, ErrUploadNotFound) {
		t.Errorf("err = %v, want ErrUploadNotFound", err)
	}
}

func TestGRPCClientDownloadFlowUploadDefaultsMaxBytesWhenUnset(t *testing.T) {
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			return &veloapi.GetTableResponse{
				Columns: []string{"Name", "Components", "Size"},
				Rows:    []*veloapi.Row{jsonRow(t, "huge.bin", []string{"a"}, 1_000_000_000)},
			}, nil
		},
	}
	vfs := &fakeVFSService{
		vfsGetBuffer: func(ctx context.Context, in *veloapi.VFSFileBuffer) (*veloapi.VFSFileBuffer, error) {
			return &veloapi.VFSFileBuffer{Data: make([]byte, in.Length)}, nil
		},
	}
	c := &grpcClient{tables: tables, vfs: vfs, timeout: time.Second, maxRows: 100}

	data, err := c.DownloadFlowUpload(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "huge.bin", 0)
	if err != nil {
		t.Fatalf("DownloadFlowUpload: %v", err)
	}
	if int64(len(data)) != defaultMaxUploadBytes {
		t.Errorf("len(data) = %d, want defaultMaxUploadBytes=%d when maxBytes<=0", len(data), defaultMaxUploadBytes)
	}
}
