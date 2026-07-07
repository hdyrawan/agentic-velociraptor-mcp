package velociraptor

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor/veloapi"
)

// fakeFlowService implements flowService for tests.
type fakeFlowService struct {
	getClientFlows  func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error)
	getFlowDetails  func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error)
	collectArtifact func(ctx context.Context, in *veloapi.ArtifactCollectorArgs) (*veloapi.ArtifactCollectorResponse, error)
	cancelFlow      func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.StartFlowResponse, error)
}

func (f *fakeFlowService) GetClientFlows(ctx context.Context, in *veloapi.GetTableRequest, opts ...grpc.CallOption) (*veloapi.GetTableResponse, error) {
	return f.getClientFlows(ctx, in)
}
func (f *fakeFlowService) GetFlowDetails(ctx context.Context, in *veloapi.ApiFlowRequest, opts ...grpc.CallOption) (*veloapi.FlowDetails, error) {
	return f.getFlowDetails(ctx, in)
}
func (f *fakeFlowService) CollectArtifact(ctx context.Context, in *veloapi.ArtifactCollectorArgs, opts ...grpc.CallOption) (*veloapi.ArtifactCollectorResponse, error) {
	return f.collectArtifact(ctx, in)
}
func (f *fakeFlowService) CancelFlow(ctx context.Context, in *veloapi.ApiFlowRequest, opts ...grpc.CallOption) (*veloapi.StartFlowResponse, error) {
	return f.cancelFlow(ctx, in)
}

// fakeTableService implements tableService for tests.
type fakeTableService struct {
	getTable func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error)
}

func (f *fakeTableService) GetTable(ctx context.Context, in *veloapi.GetTableRequest, opts ...grpc.CallOption) (*veloapi.GetTableResponse, error) {
	return f.getTable(ctx, in)
}

func jsonRow(t *testing.T, values ...any) *veloapi.Row {
	t.Helper()
	b, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return &veloapi.Row{Json: string(b)}
}

func TestGRPCClientListFlowsSuccess(t *testing.T) {
	fake := &fakeFlowService{
		getClientFlows: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			if in.ClientId != "C.1234abcd5678ef90" {
				t.Errorf("ClientId = %q", in.ClientId)
			}
			return &veloapi.GetTableResponse{
				Columns: []string{"State", "FlowId", "Artifacts", "Created"},
				Rows: []*veloapi.Row{
					jsonRow(t, "FINISHED", "F.ABC123", []string{"Windows.System.Pslist"}, 1700000000000000),
				},
			}, nil
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second, maxRows: 100}

	flows, err := c.ListFlows(context.Background(), "C.1234abcd5678ef90", 10, "")
	if err != nil {
		t.Fatalf("ListFlows: %v", err)
	}
	if len(flows) != 1 {
		t.Fatalf("len(flows) = %d, want 1", len(flows))
	}
	f := flows[0]
	if f.FlowID != "F.ABC123" || f.ClientID != "C.1234abcd5678ef90" || f.Artifact != "Windows.System.Pslist" {
		t.Errorf("flow = %+v", f)
	}
	if f.State != FlowStateFinished {
		t.Errorf("State = %q, want finished", f.State)
	}
	if f.CreatedAt == "" {
		t.Error("CreatedAt is empty")
	}
}

func TestGRPCClientListFlowsEmpty(t *testing.T) {
	fake := &fakeFlowService{
		getClientFlows: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			return &veloapi.GetTableResponse{Columns: []string{"State", "FlowId"}}, nil
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second, maxRows: 100}

	flows, err := c.ListFlows(context.Background(), "C.1234abcd5678ef90", 10, "")
	if err != nil {
		t.Fatalf("ListFlows: %v", err)
	}
	if len(flows) != 0 {
		t.Errorf("len(flows) = %d, want 0", len(flows))
	}
}

func TestGRPCClientListFlowsPagination(t *testing.T) {
	var sawStartRow uint64
	fake := &fakeFlowService{
		getClientFlows: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			sawStartRow = in.StartRow
			if in.Rows != 5 {
				t.Errorf("Rows = %d, want 5 (bounded by requested limit)", in.Rows)
			}
			return &veloapi.GetTableResponse{Columns: []string{"FlowId"}}, nil
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second, maxRows: 100}

	if _, err := c.ListFlows(context.Background(), "C.1234abcd5678ef90", 5, "offset:20"); err != nil {
		t.Fatalf("ListFlows: %v", err)
	}
	if sawStartRow != 20 {
		t.Errorf("StartRow = %d, want 20 (parsed from cursor)", sawStartRow)
	}
}

func TestGRPCClientListFlowsError(t *testing.T) {
	fake := &fakeFlowService{
		getClientFlows: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			return nil, errors.New("connection refused")
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second, maxRows: 100}

	if _, err := c.ListFlows(context.Background(), "C.1234abcd5678ef90", 5, ""); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGRPCClientGetFlowStatusSuccess(t *testing.T) {
	fake := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			if in.ClientId != "C.1234abcd5678ef90" || in.FlowId != "F.ABC123" {
				t.Errorf("request = %+v", in)
			}
			return &veloapi.FlowDetails{
				Context: &veloapi.ArtifactCollectorContext{
					SessionId: "F.ABC123",
					Request: &veloapi.ArtifactCollectorArgs{
						Artifacts: []string{"Windows.System.Pslist"},
					},
					State:      veloapi.ArtifactCollectorContext_RUNNING,
					CreateTime: 1700000000000000,
				},
			}, nil
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second}

	f, err := c.GetFlowStatus(context.Background(), "C.1234abcd5678ef90", "F.ABC123")
	if err != nil {
		t.Fatalf("GetFlowStatus: %v", err)
	}
	if f.State != FlowStateRunning {
		t.Errorf("State = %q, want running", f.State)
	}
	if f.Artifact != "Windows.System.Pslist" {
		t.Errorf("Artifact = %q", f.Artifact)
	}
}

func TestGRPCClientGetFlowStatusCancelled(t *testing.T) {
	fake := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			return &veloapi.FlowDetails{
				Context: &veloapi.ArtifactCollectorContext{
					SessionId: "F.ABC123",
					Request:   &veloapi.ArtifactCollectorArgs{Artifacts: []string{"Windows.System.Pslist"}},
					State:     veloapi.ArtifactCollectorContext_ERROR,
					Status:    "Cancelled by analyst: ",
				},
			}, nil
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second}

	f, err := c.GetFlowStatus(context.Background(), "C.1234abcd5678ef90", "F.ABC123")
	if err != nil {
		t.Fatalf("GetFlowStatus: %v", err)
	}
	if f.State != FlowStateCancelled {
		t.Errorf("State = %q, want cancelled", f.State)
	}
}

func TestGRPCClientGetFlowStatusGenuineError(t *testing.T) {
	fake := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			return &veloapi.FlowDetails{
				Context: &veloapi.ArtifactCollectorContext{
					SessionId: "F.ABC123",
					Request:   &veloapi.ArtifactCollectorArgs{Artifacts: []string{"Windows.System.Pslist"}},
					State:     veloapi.ArtifactCollectorContext_ERROR,
					Status:    "panic: divide by zero",
				},
			}, nil
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second}

	f, err := c.GetFlowStatus(context.Background(), "C.1234abcd5678ef90", "F.ABC123")
	if err != nil {
		t.Fatalf("GetFlowStatus: %v", err)
	}
	if f.State != FlowStateError {
		t.Errorf("State = %q, want error (not cancelled)", f.State)
	}
}

func TestGRPCClientGetFlowStatusNotFound(t *testing.T) {
	fake := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			return &veloapi.FlowDetails{Context: &veloapi.ArtifactCollectorContext{}}, nil
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second}

	_, err := c.GetFlowStatus(context.Background(), "C.1234abcd5678ef90", "F.NOPE")
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("err = %v, want ErrFlowNotFound", err)
	}
}

func TestGRPCClientGetFlowResultsSuccess(t *testing.T) {
	flows := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			return &veloapi.FlowDetails{
				Context: &veloapi.ArtifactCollectorContext{
					SessionId: "F.ABC123",
					Request:   &veloapi.ArtifactCollectorArgs{Artifacts: []string{"Windows.System.Pslist"}},
				},
			}, nil
		},
	}
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			if in.Artifact != "Windows.System.Pslist" {
				t.Errorf("Artifact = %q, want the flow's collected artifact", in.Artifact)
			}
			return &veloapi.GetTableResponse{
				Columns:   []string{"Pid", "Name"},
				Rows:      []*veloapi.Row{jsonRow(t, 1234, "explorer.exe")},
				TotalRows: 1,
			}, nil
		},
	}
	c := &grpcClient{flows: flows, tables: tables, timeout: time.Second, maxRows: 100}

	page, err := c.GetFlowResults(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "", 10, 1<<20, "")
	if err != nil {
		t.Fatalf("GetFlowResults: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0]["Name"] != "explorer.exe" {
		t.Errorf("Rows = %+v", page.Rows)
	}
}

func TestGRPCClientGetFlowResultsNotFound(t *testing.T) {
	flows := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			return &veloapi.FlowDetails{Context: &veloapi.ArtifactCollectorContext{}}, nil
		},
	}
	c := &grpcClient{flows: flows, timeout: time.Second, maxRows: 100}

	_, err := c.GetFlowResults(context.Background(), "C.1234abcd5678ef90", "F.NOPE", "", 10, 1<<20, "")
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("err = %v, want ErrFlowNotFound", err)
	}
}

func TestGRPCClientGetFlowResultsPaginationTruncation(t *testing.T) {
	flows := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			return &veloapi.FlowDetails{
				Context: &veloapi.ArtifactCollectorContext{
					SessionId: "F.ABC123",
					Request:   &veloapi.ArtifactCollectorArgs{Artifacts: []string{"Windows.System.Pslist"}},
				},
			}, nil
		},
	}
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			if in.Rows != 2 {
				t.Errorf("Rows = %d, want 2", in.Rows)
			}
			return &veloapi.GetTableResponse{
				Columns:   []string{"Pid"},
				Rows:      []*veloapi.Row{jsonRow(t, 1), jsonRow(t, 2)},
				TotalRows: 5,
			}, nil
		},
	}
	c := &grpcClient{flows: flows, tables: tables, timeout: time.Second, maxRows: 100}

	page, err := c.GetFlowResults(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "", 2, 1<<20, "")
	if err != nil {
		t.Fatalf("GetFlowResults: %v", err)
	}
	if !page.Truncated {
		t.Error("Truncated = false, want true (2 of 5 total rows returned)")
	}
	if page.NextCursor != "offset:2" {
		t.Errorf("NextCursor = %q, want offset:2", page.NextCursor)
	}
}

// TestGRPCClientGetFlowResultsNamedSourceRequiresSelection fixes the
// v0.10.3 named-source bug: a flow whose artifact
// (Generic.Client.Info-shaped fixture) compiles to more than one named
// Velociraptor result source must not silently return zero rows; it
// must report SourceRequired with the real source names instead. See
// docs/live-validation-report-v0.10.2.md finding 2.
func TestGRPCClientGetFlowResultsNamedSourceRequiresSelection(t *testing.T) {
	flows := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			return &veloapi.FlowDetails{
				Context: &veloapi.ArtifactCollectorContext{
					SessionId: "F.ABC123",
					Request:   &veloapi.ArtifactCollectorArgs{Artifacts: []string{"Generic.Client.Info"}},
				},
			}, nil
		},
	}
	artifacts := &fakeArtifactCatalog{
		resp: &veloapi.ArtifactDescriptors{
			Items: []*veloapi.Artifact{{
				Name: "Generic.Client.Info",
				Sources: []*veloapi.ArtifactSource{
					{Name: "BasicInformation"},
					{Name: "DetailedInfo"},
					{Name: "LinuxInfo"},
				},
			}},
		},
	}
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			t.Fatal("GetTable should not be called when a source must first be selected")
			return nil, nil
		},
	}
	c := &grpcClient{flows: flows, tables: tables, artifacts: artifacts, timeout: time.Second, maxRows: 100}

	page, err := c.GetFlowResults(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "", 10, 1<<20, "")
	if err != nil {
		t.Fatalf("GetFlowResults: %v", err)
	}
	if !page.SourceRequired {
		t.Fatal("SourceRequired = false, want true for a multi-source artifact with no source selected")
	}
	want := []string{"BasicInformation", "DetailedInfo", "LinuxInfo"}
	if len(page.AvailableSources) != len(want) {
		t.Fatalf("AvailableSources = %v, want %v", page.AvailableSources, want)
	}
	if len(page.Rows) != 0 {
		t.Errorf("Rows = %+v, want none when source selection is required", page.Rows)
	}
}

// TestGRPCClientGetFlowResultsNamedSourceSelected proves the actual fix:
// with an explicit source, GetTable is called with the source-qualified
// "Artifact/Source" name (confirmed against real Velociraptor's
// paths.SplitFullSourceName convention) and real rows come back —
// this is the exact case that previously silently returned zero rows.
func TestGRPCClientGetFlowResultsNamedSourceSelected(t *testing.T) {
	flows := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			return &veloapi.FlowDetails{
				Context: &veloapi.ArtifactCollectorContext{
					SessionId: "F.ABC123",
					Request:   &veloapi.ArtifactCollectorArgs{Artifacts: []string{"Generic.Client.Info"}},
				},
			}, nil
		},
	}
	artifacts := &fakeArtifactCatalog{
		resp: &veloapi.ArtifactDescriptors{
			Items: []*veloapi.Artifact{{
				Name: "Generic.Client.Info",
				Sources: []*veloapi.ArtifactSource{
					{Name: "BasicInformation"},
					{Name: "DetailedInfo"},
				},
			}},
		},
	}
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			if in.Artifact != "Generic.Client.Info/BasicInformation" {
				t.Errorf("Artifact = %q, want source-qualified %q", in.Artifact, "Generic.Client.Info/BasicInformation")
			}
			return &veloapi.GetTableResponse{
				Columns:   []string{"Hostname"},
				Rows:      []*veloapi.Row{jsonRow(t, "mcp-lab-linux-01")},
				TotalRows: 1,
			}, nil
		},
	}
	c := &grpcClient{flows: flows, tables: tables, artifacts: artifacts, timeout: time.Second, maxRows: 100}

	page, err := c.GetFlowResults(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "BasicInformation", 10, 1<<20, "")
	if err != nil {
		t.Fatalf("GetFlowResults: %v", err)
	}
	if page.SourceRequired {
		t.Fatal("SourceRequired = true, want false when an explicit source was given")
	}
	if len(page.Rows) != 1 || page.Rows[0]["Hostname"] != "mcp-lab-linux-01" {
		t.Errorf("Rows = %+v", page.Rows)
	}
}

// TestGRPCClientGetFlowResultsUnknownSourceRejected confirms an invalid
// caller-supplied source is rejected with ErrUnknownResultSource rather
// than silently querying a nonexistent table.
func TestGRPCClientGetFlowResultsUnknownSourceRejected(t *testing.T) {
	flows := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			return &veloapi.FlowDetails{
				Context: &veloapi.ArtifactCollectorContext{
					SessionId: "F.ABC123",
					Request:   &veloapi.ArtifactCollectorArgs{Artifacts: []string{"Generic.Client.Info"}},
				},
			}, nil
		},
	}
	artifacts := &fakeArtifactCatalog{
		resp: &veloapi.ArtifactDescriptors{
			Items: []*veloapi.Artifact{{
				Name:    "Generic.Client.Info",
				Sources: []*veloapi.ArtifactSource{{Name: "BasicInformation"}},
			}},
		},
	}
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			t.Fatal("GetTable should not be called for an unknown source")
			return nil, nil
		},
	}
	c := &grpcClient{flows: flows, tables: tables, artifacts: artifacts, timeout: time.Second, maxRows: 100}

	_, err := c.GetFlowResults(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "NotARealSource", 10, 1<<20, "")
	if !errors.Is(err, ErrUnknownResultSource) {
		t.Errorf("err = %v, want wrapping ErrUnknownResultSource", err)
	}
}

// TestGRPCClientGetFlowResultsDefaultSourceUnaffected is a regression
// guard: a single-unnamed-source artifact (the pre-v0.10.3 common case,
// e.g. Linux.Sys.Pslist/Windows.System.Pslist) must keep querying the
// bare artifact name, unaffected by source resolution.
func TestGRPCClientGetFlowResultsDefaultSourceUnaffected(t *testing.T) {
	flows := &fakeFlowService{
		getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
			return &veloapi.FlowDetails{
				Context: &veloapi.ArtifactCollectorContext{
					SessionId: "F.ABC123",
					Request:   &veloapi.ArtifactCollectorArgs{Artifacts: []string{"Linux.Sys.Pslist"}},
				},
			}, nil
		},
	}
	artifacts := &fakeArtifactCatalog{
		resp: &veloapi.ArtifactDescriptors{
			Items: []*veloapi.Artifact{{
				Name:    "Linux.Sys.Pslist",
				Sources: []*veloapi.ArtifactSource{{Name: ""}},
			}},
		},
	}
	tables := &fakeTableService{
		getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
			if in.Artifact != "Linux.Sys.Pslist" {
				t.Errorf("Artifact = %q, want bare %q (unnamed source)", in.Artifact, "Linux.Sys.Pslist")
			}
			return &veloapi.GetTableResponse{
				Columns:   []string{"Pid"},
				Rows:      []*veloapi.Row{jsonRow(t, 1)},
				TotalRows: 1,
			}, nil
		},
	}
	c := &grpcClient{flows: flows, tables: tables, artifacts: artifacts, timeout: time.Second, maxRows: 100}

	page, err := c.GetFlowResults(context.Background(), "C.1234abcd5678ef90", "F.ABC123", "", 10, 1<<20, "")
	if err != nil {
		t.Fatalf("GetFlowResults: %v", err)
	}
	if page.SourceRequired {
		t.Fatal("SourceRequired = true, want false for a single-unnamed-source artifact")
	}
	if len(page.Rows) != 1 {
		t.Errorf("Rows = %+v, want 1 row", page.Rows)
	}
}

func TestGRPCClientCollectArtifactSendsArtifactsAndSpecs(t *testing.T) {
	fake := &fakeFlowService{
		collectArtifact: func(ctx context.Context, in *veloapi.ArtifactCollectorArgs) (*veloapi.ArtifactCollectorResponse, error) {
			if in.ClientId != "C.1234abcd5678ef90" {
				t.Errorf("ClientId = %q", in.ClientId)
			}
			if len(in.Artifacts) != 1 || in.Artifacts[0] != "Windows.System.Pslist" {
				t.Errorf("Artifacts = %v, want the legacy name list populated too", in.Artifacts)
			}
			if len(in.Specs) != 1 || in.Specs[0].GetArtifact() != "Windows.System.Pslist" {
				t.Errorf("Specs = %+v", in.Specs)
			}
			env := in.Specs[0].GetParameters().GetEnv()
			if len(env) != 1 || env[0].GetKey() != "Pid" || env[0].GetValue() != "1234" {
				t.Errorf("Env = %+v, want [{Pid 1234}]", env)
			}
			return &veloapi.ArtifactCollectorResponse{FlowId: "F.NEW1"}, nil
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second}

	summary, err := c.CollectArtifact(context.Background(), CollectionRequest{
		ClientID: "C.1234abcd5678ef90", Artifact: "Windows.System.Pslist", Parameters: map[string]string{"Pid": "1234"},
	})
	if err != nil {
		t.Fatalf("CollectArtifact: %v", err)
	}
	if summary.FlowID != "F.NEW1" || summary.State != FlowStateRunning {
		t.Errorf("summary = %+v", summary)
	}
}

func TestGRPCClientCollectArtifactError(t *testing.T) {
	fake := &fakeFlowService{
		collectArtifact: func(ctx context.Context, in *veloapi.ArtifactCollectorArgs) (*veloapi.ArtifactCollectorResponse, error) {
			return nil, errors.New("permission denied")
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second}

	if _, err := c.CollectArtifact(context.Background(), CollectionRequest{ClientID: "C.1234abcd5678ef90", Artifact: "X"}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGRPCClientCancelFlowSuccess(t *testing.T) {
	var sawReq *veloapi.ApiFlowRequest
	fake := &fakeFlowService{
		cancelFlow: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.StartFlowResponse, error) {
			sawReq = in
			return &veloapi.StartFlowResponse{FlowId: in.FlowId}, nil
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second}

	if err := c.CancelFlow(context.Background(), "C.1234abcd5678ef90", "F.ABC123"); err != nil {
		t.Fatalf("CancelFlow: %v", err)
	}
	if sawReq.ClientId != "C.1234abcd5678ef90" || sawReq.FlowId != "F.ABC123" {
		t.Errorf("request = %+v", sawReq)
	}
}

func TestGRPCClientCancelFlowError(t *testing.T) {
	fake := &fakeFlowService{
		cancelFlow: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.StartFlowResponse, error) {
			return nil, errors.New("not found")
		},
	}
	c := &grpcClient{flows: fake, timeout: time.Second}

	if err := c.CancelFlow(context.Background(), "C.1234abcd5678ef90", "F.NOPE"); err == nil {
		t.Fatal("expected error, got nil")
	}
}
