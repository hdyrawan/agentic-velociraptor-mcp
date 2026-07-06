package mcpserver

import (
	"context"
	"testing"

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
