package velociraptor

import (
	"context"
	"errors"
)

// ErrUploadNotFound is returned by GetFlowUploadMetadata and
// DownloadFlowUpload when the named upload does not exist on the given
// client/flow, so callers can distinguish "no such upload" from any
// other connectivity/RPC failure.
var ErrUploadNotFound = errors.New("velociraptor: upload not found")

// UploadSummary describes one file uploaded to the server as part of a
// flow's results (e.g. a collected memory sample or exported file),
// without its content.
type UploadSummary struct {
	Component string
	Name      string
	SizeBytes int64
	Hash      string
}

// UploadReader backs velo_list_flow_uploads and
// velo_get_flow_upload_metadata. Listing/metadata are read-only and do
// not require approval; fetching content does.
type UploadReader interface {
	ListFlowUploads(ctx context.Context, clientID, flowID string) ([]UploadSummary, error)
	GetFlowUploadMetadata(ctx context.Context, clientID, flowID, uploadName string) (UploadSummary, error)
}

func (placeholderClient) ListFlowUploads(ctx context.Context, clientID, flowID string) ([]UploadSummary, error) {
	return nil, ErrNotImplemented
}

func (placeholderClient) GetFlowUploadMetadata(ctx context.Context, clientID, flowID, uploadName string) (UploadSummary, error) {
	return UploadSummary{}, ErrNotImplemented
}

// UploadDownloader backs velo_download_flow_upload_with_approval. This
// is a raw-evidence-disclosure operation and must always require
// approval, always enforce config.VelociraptorConfig.MaxUploadBytes, and
// must use the write API identity path even though it is conceptually a
// "read" of already-collected data, since it is the single riskiest data
// exfiltration primitive this server exposes.
type UploadDownloader interface {
	DownloadFlowUpload(ctx context.Context, clientID, flowID, uploadName string, maxBytes int64) ([]byte, error)
}

func (placeholderClient) DownloadFlowUpload(ctx context.Context, clientID, flowID, uploadName string, maxBytes int64) ([]byte, error) {
	return nil, ErrNotImplemented
}
