package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

const (
	testClientID = "C.1234abcd5678ef90"
	testFlowID   = "F.BN2HJC4N4T6KG"
)

type fakeFlowClient struct {
	velociraptor.Client
	listFlows      func(ctx context.Context, clientID string, limit int, cursor string) ([]velociraptor.FlowSummary, error)
	getFlowStatus  func(ctx context.Context, clientID, flowID string) (velociraptor.FlowSummary, error)
	getFlowResults func(ctx context.Context, clientID, flowID string, maxRows int, maxBytes int64, cursor string) (velociraptor.FlowResultPage, error)
	collectCalls   int
	cancelCalls    int
}

func (f *fakeFlowClient) ListFlows(ctx context.Context, clientID string, limit int, cursor string) ([]velociraptor.FlowSummary, error) {
	return f.listFlows(ctx, clientID, limit, cursor)
}

func (f *fakeFlowClient) GetFlowStatus(ctx context.Context, clientID, flowID string) (velociraptor.FlowSummary, error) {
	return f.getFlowStatus(ctx, clientID, flowID)
}

func (f *fakeFlowClient) GetFlowResults(ctx context.Context, clientID, flowID string, maxRows int, maxBytes int64, cursor string) (velociraptor.FlowResultPage, error) {
	return f.getFlowResults(ctx, clientID, flowID, maxRows, maxBytes, cursor)
}

func (f *fakeFlowClient) CollectArtifact(ctx context.Context, req velociraptor.CollectionRequest) (velociraptor.FlowSummary, error) {
	f.collectCalls++
	return velociraptor.FlowSummary{}, velociraptor.ErrNotImplemented
}

func (f *fakeFlowClient) CancelFlow(ctx context.Context, clientID, flowID string) error {
	f.cancelCalls++
	return velociraptor.ErrNotImplemented
}

func TestListFlowsHandlerRealModeSuccess(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	fake := &fakeFlowClient{Client: velociraptor.NewClient()}
	fake.listFlows = func(ctx context.Context, clientID string, limit int, cursor string) ([]velociraptor.FlowSummary, error) {
		if clientID != testClientID {
			t.Fatalf("clientID = %q", clientID)
		}
		if limit != 2 {
			t.Fatalf("limit = %d, want 2", limit)
		}
		return []velociraptor.FlowSummary{{FlowID: testFlowID, ClientID: clientID, Artifact: "Windows.System.Pslist", State: velociraptor.FlowStateFinished}}, nil
	}
	deps.ReadClient = fake

	_, out, err := newListFlowsHandler(deps)(context.Background(), nil, ListFlowsInput{ClientID: testClientID, Limit: 2})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess || out.Mode != VelociraptorModeReal {
		t.Fatalf("out = %+v, want success real", out)
	}
	if len(out.Flows) != 1 || out.Flows[0].FlowID != testFlowID {
		t.Fatalf("Flows = %+v", out.Flows)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess || evt.RowCount != 1 || evt.ClientID != testClientID {
		t.Errorf("audit event = %+v, ok=%v", evt, ok)
	}
	if fake.collectCalls != 0 || fake.cancelCalls != 0 {
		t.Fatalf("write methods called: collect=%d cancel=%d", fake.collectCalls, fake.cancelCalls)
	}
}

func TestListFlowsHandlerRealModeEmpty(t *testing.T) {
	deps, _ := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeFlowClient{Client: velociraptor.NewClient(), listFlows: func(ctx context.Context, clientID string, limit int, cursor string) ([]velociraptor.FlowSummary, error) {
		return nil, nil
	}}

	_, out, err := newListFlowsHandler(deps)(context.Background(), nil, ListFlowsInput{ClientID: testClientID})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusEmpty {
		t.Fatalf("Status = %q, want empty", out.Status)
	}
	if len(out.Flows) != 0 {
		t.Fatalf("Flows = %+v, want empty", out.Flows)
	}
}

func TestGetFlowStatusHandlerRealModeSuccess(t *testing.T) {
	deps, _ := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeFlowClient{Client: velociraptor.NewClient(), getFlowStatus: func(ctx context.Context, clientID, flowID string) (velociraptor.FlowSummary, error) {
		return velociraptor.FlowSummary{FlowID: flowID, ClientID: clientID, Artifact: "Generic.Client.Info", State: velociraptor.FlowStateRunning}, nil
	}}

	_, out, err := newGetFlowStatusHandler(deps)(context.Background(), nil, GetFlowStatusInput{ClientID: testClientID, FlowID: testFlowID})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess || out.Flow == nil || out.Flow.State != "running" {
		t.Fatalf("out = %+v", out)
	}
}

func TestGetFlowStatusHandlerNotFound(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeFlowClient{Client: velociraptor.NewClient(), getFlowStatus: func(ctx context.Context, clientID, flowID string) (velociraptor.FlowSummary, error) {
		return velociraptor.FlowSummary{}, velociraptor.ErrFlowNotFound
	}}

	_, out, err := newGetFlowStatusHandler(deps)(context.Background(), nil, GetFlowStatusInput{ClientID: testClientID, FlowID: testFlowID})
	if err != nil {
		t.Fatalf("handler returned Go error, want structured not_found: %v", err)
	}
	if out.Status != response.StatusNotFound {
		t.Fatalf("Status = %q, want not_found", out.Status)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError || evt.FlowID != testFlowID {
		t.Errorf("audit event = %+v, ok=%v", evt, ok)
	}
}

func TestGetFlowResultsHandlerRealModeSuccess(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeFlowClient{Client: velociraptor.NewClient(), getFlowResults: func(ctx context.Context, clientID, flowID string, maxRows int, maxBytes int64, cursor string) (velociraptor.FlowResultPage, error) {
		if maxRows != 2 {
			t.Fatalf("maxRows = %d, want 2", maxRows)
		}
		if maxBytes != deps.Config.Velociraptor.MaxResultBytes {
			t.Fatalf("maxBytes = %d", maxBytes)
		}
		return velociraptor.FlowResultPage{Rows: []map[string]any{{"pid": 1}, {"pid": 2}}, TotalRows: 2}, nil
	}}

	_, out, err := newGetFlowResultsHandler(deps)(context.Background(), nil, GetFlowResultsInput{ClientID: testClientID, FlowID: testFlowID, Limit: 2})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess || out.ReturnedRows != 2 || len(out.Rows) != 2 {
		t.Fatalf("out = %+v", out)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess || evt.RowCount != 2 || evt.ByteCount == 0 {
		t.Errorf("audit event = %+v, ok=%v", evt, ok)
	}
}

func TestGetFlowResultsHandlerEmpty(t *testing.T) {
	deps, _ := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeFlowClient{Client: velociraptor.NewClient(), getFlowResults: func(ctx context.Context, clientID, flowID string, maxRows int, maxBytes int64, cursor string) (velociraptor.FlowResultPage, error) {
		return velociraptor.FlowResultPage{}, nil
	}}

	_, out, err := newGetFlowResultsHandler(deps)(context.Background(), nil, GetFlowResultsInput{ClientID: testClientID, FlowID: testFlowID})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusEmpty || len(out.Rows) != 0 {
		t.Fatalf("out = %+v, want empty", out)
	}
}

func TestGetFlowResultsHandlerNotFound(t *testing.T) {
	deps, _ := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeFlowClient{Client: velociraptor.NewClient(), getFlowResults: func(ctx context.Context, clientID, flowID string, maxRows int, maxBytes int64, cursor string) (velociraptor.FlowResultPage, error) {
		return velociraptor.FlowResultPage{}, velociraptor.ErrFlowNotFound
	}}

	_, out, err := newGetFlowResultsHandler(deps)(context.Background(), nil, GetFlowResultsInput{ClientID: testClientID, FlowID: testFlowID})
	if err != nil {
		t.Fatalf("handler returned Go error, want structured not_found: %v", err)
	}
	if out.Status != response.StatusNotFound {
		t.Fatalf("Status = %q, want not_found", out.Status)
	}
}

func TestGetFlowResultsHandlerEnforcesLimitAndBytes(t *testing.T) {
	deps, _ := testDeps(t)
	deps.Config.Velociraptor.MaxRows = 2
	deps.Config.Velociraptor.MaxResultBytes = 20
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeFlowClient{Client: velociraptor.NewClient(), getFlowResults: func(ctx context.Context, clientID, flowID string, maxRows int, maxBytes int64, cursor string) (velociraptor.FlowResultPage, error) {
		if maxRows != 2 || maxBytes != 20 {
			t.Fatalf("limits passed to client = rows %d bytes %d, want 2/20", maxRows, maxBytes)
		}
		return velociraptor.FlowResultPage{Rows: []map[string]any{{"a": "1"}, {"b": "2"}, {"c": "3"}}, TotalRows: 3}, nil
	}}

	_, out, err := newGetFlowResultsHandler(deps)(context.Background(), nil, GetFlowResultsInput{ClientID: testClientID, FlowID: testFlowID, Limit: 99})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(out.Rows) > 2 {
		t.Fatalf("returned %d rows, want <= 2", len(out.Rows))
	}
	if out.ByteCount > 20 {
		t.Fatalf("ByteCount = %d, want <= 20", out.ByteCount)
	}
	if !out.Truncated || out.NextCursor == "" {
		t.Fatalf("out = %+v, want truncated with next_cursor", out)
	}
}

func TestFlowHandlersRejectInvalidClientIDAndFlowID(t *testing.T) {
	deps, sink := testDeps(t)

	if _, _, err := newListFlowsHandler(deps)(context.Background(), nil, ListFlowsInput{ClientID: "bad"}); err == nil {
		t.Fatal("list flows accepted invalid client_id")
	}
	if evt, ok := sink.last(); !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Fatalf("audit after invalid client = %+v ok=%v", evt, ok)
	}

	if _, _, err := newGetFlowStatusHandler(deps)(context.Background(), nil, GetFlowStatusInput{ClientID: testClientID, FlowID: "bad"}); err == nil {
		t.Fatal("get flow status accepted invalid flow_id")
	}
	if evt, ok := sink.last(); !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Fatalf("audit after invalid flow = %+v ok=%v", evt, ok)
	}

	if _, _, err := newGetFlowResultsHandler(deps)(context.Background(), nil, GetFlowResultsInput{ClientID: testClientID, FlowID: "bad"}); err == nil {
		t.Fatal("get flow results accepted invalid flow_id")
	}
}

func TestFlowHandlersDoNotCallWriteMethods(t *testing.T) {
	deps, _ := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	fake := &fakeFlowClient{Client: velociraptor.NewClient()}
	fake.listFlows = func(ctx context.Context, clientID string, limit int, cursor string) ([]velociraptor.FlowSummary, error) {
		return nil, nil
	}
	fake.getFlowStatus = func(ctx context.Context, clientID, flowID string) (velociraptor.FlowSummary, error) {
		return velociraptor.FlowSummary{FlowID: flowID, ClientID: clientID}, nil
	}
	fake.getFlowResults = func(ctx context.Context, clientID, flowID string, maxRows int, maxBytes int64, cursor string) (velociraptor.FlowResultPage, error) {
		return velociraptor.FlowResultPage{}, nil
	}
	deps.ReadClient = fake

	_, _, _ = newListFlowsHandler(deps)(context.Background(), nil, ListFlowsInput{ClientID: testClientID})
	_, _, _ = newGetFlowStatusHandler(deps)(context.Background(), nil, GetFlowStatusInput{ClientID: testClientID, FlowID: testFlowID})
	_, _, _ = newGetFlowResultsHandler(deps)(context.Background(), nil, GetFlowResultsInput{ClientID: testClientID, FlowID: testFlowID})

	if fake.collectCalls != 0 || fake.cancelCalls != 0 {
		t.Fatalf("write methods called: collect=%d cancel=%d", fake.collectCalls, fake.cancelCalls)
	}
}

// --- v0.4.0: velo_list_flow_uploads ---

func TestListFlowUploadsMockMode(t *testing.T) {
	deps, _ := testDeps(t)
	handler := newListFlowUploadsHandler(deps)

	_, out, err := handler(context.Background(), nil, ListFlowUploadsInput{ClientID: testCollectClientID, FlowID: testFlowID})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Mode != VelociraptorModeMock || out.Status != response.StatusSuccess {
		t.Errorf("out = %+v, want mock mode success", out)
	}
}

func TestListFlowUploadsInvalidInput(t *testing.T) {
	deps, _ := testDeps(t)
	handler := newListFlowUploadsHandler(deps)

	if _, _, err := handler(context.Background(), nil, ListFlowUploadsInput{ClientID: "bad", FlowID: testFlowID}); err == nil {
		t.Error("expected error for invalid client id")
	}
	if _, _, err := handler(context.Background(), nil, ListFlowUploadsInput{ClientID: testCollectClientID, FlowID: "bad"}); err == nil {
		t.Error("expected error for invalid flow id")
	}
}

func TestListFlowUploadsRealModeSuccess(t *testing.T) {
	deps, _ := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVelociraptorClient{
		listFlowUploads: func(ctx context.Context, clientID, flowID string) ([]velociraptor.UploadSummary, error) {
			return []velociraptor.UploadSummary{{Component: "file", Name: "memory.dmp", SizeBytes: 1024, Hash: "abc"}}, nil
		},
	}
	handler := newListFlowUploadsHandler(deps)

	_, out, err := handler(context.Background(), nil, ListFlowUploadsInput{ClientID: testCollectClientID, FlowID: testFlowID})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusSuccess || len(out.Uploads) != 1 || out.Uploads[0].Name != "memory.dmp" {
		t.Errorf("out = %+v, want one upload named memory.dmp", out)
	}
}

// --- v0.4.0: velo_get_flow_upload_metadata ---

func TestGetFlowUploadMetadataNotFound(t *testing.T) {
	deps, _ := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVelociraptorClient{
		getUploadMetadata: func(ctx context.Context, clientID, flowID, uploadName string) (velociraptor.UploadSummary, error) {
			return velociraptor.UploadSummary{}, velociraptor.ErrUploadNotFound
		},
	}
	handler := newGetFlowUploadMetadataHandler(deps)

	_, out, err := handler(context.Background(), nil, GetFlowUploadMetadataInput{ClientID: testCollectClientID, FlowID: testFlowID, UploadName: "missing.bin"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusNotFound {
		t.Errorf("Status = %q, want not_found", out.Status)
	}
}

func TestGetFlowUploadMetadataSuccess(t *testing.T) {
	deps, _ := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVelociraptorClient{
		getUploadMetadata: func(ctx context.Context, clientID, flowID, uploadName string) (velociraptor.UploadSummary, error) {
			return velociraptor.UploadSummary{Component: "file", Name: uploadName, SizeBytes: 42, Hash: "deadbeef"}, nil
		},
	}
	handler := newGetFlowUploadMetadataHandler(deps)

	_, out, err := handler(context.Background(), nil, GetFlowUploadMetadataInput{ClientID: testCollectClientID, FlowID: testFlowID, UploadName: "memory.dmp"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusSuccess || out.Upload == nil || out.Upload.SizeBytes != 42 {
		t.Errorf("out = %+v, want success with SizeBytes=42", out)
	}
}

// --- v0.4.0: velo_download_flow_upload_with_approval ---

func testDownloadPilotDeps(t *testing.T) (Deps, *fakeAuditSink) {
	t.Helper()
	deps, sink := testWritePilotDeps(t)
	deps.Config.Velociraptor.DownloadDir = t.TempDir()
	deps.Config.Velociraptor.MaxUploadBytes = 1048576
	return deps, sink
}

func TestDownloadFlowUploadDisabledWithoutDownloadDir(t *testing.T) {
	deps, _ := testWritePilotDeps(t) // DownloadDir left empty.
	handler := newDownloadFlowUploadHandler(deps)

	_, out, err := handler(context.Background(), nil, DownloadFlowUploadInput{
		ClientID: testCollectClientID, FlowID: testFlowID, UploadName: "memory.dmp",
		CaseID: "CASE-1", Reason: "evidence", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err == nil {
		t.Fatalf("expected error when download_dir is unset, got out=%+v", out)
	}
	if !strings.Contains(err.Error(), "download_dir") {
		t.Errorf("error = %v, want mention of download_dir", err)
	}
}

func TestDownloadFlowUploadApprovedFakeExecutionSucceeds(t *testing.T) {
	deps, sink := testDownloadPilotDeps(t)
	const uploadName = "memory.dmp"
	payload := []byte("fake evidence bytes")
	deps.WriteClient = &fakeVelociraptorClient{
		downloadFlowUpload: func(ctx context.Context, clientID, flowID, name string, maxBytes int64) ([]byte, error) {
			if clientID != testCollectClientID || flowID != testFlowID || name != uploadName {
				t.Fatalf("unexpected DownloadFlowUpload call: %s %s %s", clientID, flowID, name)
			}
			return payload, nil
		},
	}
	handler := newDownloadFlowUploadHandler(deps)

	req := approval.Request{
		ID: "ref-1", Operation: approval.OperationDownloadFlowUpload,
		CaseID: "CASE-1", Reason: "evidence", Requester: "analyst",
		ClientID: testCollectClientID, FlowID: testFlowID, UploadName: uploadName,
	}
	approveRequest(t, deps.Approvals, req)

	_, out, err := handler(context.Background(), nil, DownloadFlowUploadInput{
		ClientID: testCollectClientID, FlowID: testFlowID, UploadName: uploadName,
		CaseID: "CASE-1", Reason: "evidence", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Fatalf("Status = %q, want success: %+v", out.Status, out)
	}
	if out.SizeBytes != int64(len(payload)) {
		t.Errorf("SizeBytes = %d, want %d", out.SizeBytes, len(payload))
	}
	if out.LocalPath == "" {
		t.Fatal("LocalPath is empty")
	}

	// The response must never carry raw bytes; only metadata.
	written, err := os.ReadFile(out.LocalPath)
	if err != nil {
		t.Fatalf("read local path: %v", err)
	}
	if string(written) != string(payload) {
		t.Errorf("written file content = %q, want %q", written, payload)
	}
	if !strings.HasPrefix(out.LocalPath, deps.Config.Velociraptor.DownloadDir) {
		t.Errorf("LocalPath %q not under configured download_dir %q", out.LocalPath, deps.Config.Velociraptor.DownloadDir)
	}
	// The filename must not embed the caller-supplied upload_name.
	if strings.Contains(filepath.Base(out.LocalPath), uploadName) {
		t.Errorf("local filename %q unexpectedly embeds upload_name", filepath.Base(out.LocalPath))
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != "success" || evt.ByteCount != int64(len(payload)) {
		t.Errorf("audit event = %+v, ok=%v, want success with ByteCount=%d", evt, ok, len(payload))
	}
}

func TestDownloadFlowUploadNotFound(t *testing.T) {
	deps, _ := testDownloadPilotDeps(t)
	deps.WriteClient = &fakeVelociraptorClient{
		downloadFlowUpload: func(ctx context.Context, clientID, flowID, name string, maxBytes int64) ([]byte, error) {
			return nil, velociraptor.ErrUploadNotFound
		},
	}
	handler := newDownloadFlowUploadHandler(deps)

	req := approval.Request{
		ID: "ref-1", Operation: approval.OperationDownloadFlowUpload,
		CaseID: "CASE-1", Reason: "evidence", Requester: "analyst",
		ClientID: testCollectClientID, FlowID: testFlowID, UploadName: "missing.bin",
	}
	approveRequest(t, deps.Approvals, req)

	_, out, err := handler(context.Background(), nil, DownloadFlowUploadInput{
		ClientID: testCollectClientID, FlowID: testFlowID, UploadName: "missing.bin",
		CaseID: "CASE-1", Reason: "evidence", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusNotFound {
		t.Errorf("Status = %q, want not_found", out.Status)
	}
}
