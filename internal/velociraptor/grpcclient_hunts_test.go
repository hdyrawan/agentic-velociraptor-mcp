package velociraptor

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor/veloapi"
)

// fakeHuntService implements huntService for tests.
type fakeHuntService struct {
	createHunt     func(ctx context.Context, in *veloapi.Hunt) (*veloapi.StartFlowResponse, error)
	modifyHunt     func(ctx context.Context, in *veloapi.HuntMutation) (*emptypb.Empty, error)
	listHunts      func(ctx context.Context, in *veloapi.ListHuntsRequest) (*veloapi.ListHuntsResponse, error)
	getHunt        func(ctx context.Context, in *veloapi.GetHuntRequest) (*veloapi.Hunt, error)
	getHuntResults func(ctx context.Context, in *veloapi.GetHuntResultsRequest) (*veloapi.GetTableResponse, error)
	estimateHunt   func(ctx context.Context, in *veloapi.HuntEstimateRequest) (*veloapi.HuntStats, error)
}

func (f *fakeHuntService) CreateHunt(ctx context.Context, in *veloapi.Hunt, opts ...grpc.CallOption) (*veloapi.StartFlowResponse, error) {
	return f.createHunt(ctx, in)
}
func (f *fakeHuntService) ModifyHunt(ctx context.Context, in *veloapi.HuntMutation, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return f.modifyHunt(ctx, in)
}
func (f *fakeHuntService) ListHunts(ctx context.Context, in *veloapi.ListHuntsRequest, opts ...grpc.CallOption) (*veloapi.ListHuntsResponse, error) {
	return f.listHunts(ctx, in)
}
func (f *fakeHuntService) GetHunt(ctx context.Context, in *veloapi.GetHuntRequest, opts ...grpc.CallOption) (*veloapi.Hunt, error) {
	return f.getHunt(ctx, in)
}
func (f *fakeHuntService) GetHuntResults(ctx context.Context, in *veloapi.GetHuntResultsRequest, opts ...grpc.CallOption) (*veloapi.GetTableResponse, error) {
	return f.getHuntResults(ctx, in)
}
func (f *fakeHuntService) EstimateHunt(ctx context.Context, in *veloapi.HuntEstimateRequest, opts ...grpc.CallOption) (*veloapi.HuntStats, error) {
	return f.estimateHunt(ctx, in)
}

func TestGRPCClientListHuntsSuccess(t *testing.T) {
	fake := &fakeHuntService{
		listHunts: func(ctx context.Context, in *veloapi.ListHuntsRequest) (*veloapi.ListHuntsResponse, error) {
			return &veloapi.ListHuntsResponse{
				Items: []*veloapi.Hunt{
					{HuntId: "H.1", Artifacts: []string{"Windows.System.Pslist"}, State: veloapi.Hunt_RUNNING, CreateTime: 1700000000000000},
				},
			}, nil
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second, maxRows: 100}

	hunts, err := c.ListHunts(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListHunts: %v", err)
	}
	if len(hunts) != 1 || hunts[0].HuntID != "H.1" || hunts[0].Artifact != "Windows.System.Pslist" || hunts[0].State != HuntStateRunning {
		t.Errorf("hunts = %+v", hunts)
	}
}

func TestGRPCClientListHuntsError(t *testing.T) {
	fake := &fakeHuntService{
		listHunts: func(ctx context.Context, in *veloapi.ListHuntsRequest) (*veloapi.ListHuntsResponse, error) {
			return nil, errors.New("connection refused")
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second, maxRows: 100}

	if _, err := c.ListHunts(context.Background(), 10); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGRPCClientGetHuntStatusSuccess(t *testing.T) {
	fake := &fakeHuntService{
		getHunt: func(ctx context.Context, in *veloapi.GetHuntRequest) (*veloapi.Hunt, error) {
			if in.HuntId != "H.1" {
				t.Errorf("HuntId = %q", in.HuntId)
			}
			return &veloapi.Hunt{HuntId: "H.1", State: veloapi.Hunt_STOPPED, Stats: &veloapi.HuntStats{TotalClientsScheduled: 7}}, nil
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	h, err := c.GetHuntStatus(context.Background(), "H.1")
	if err != nil {
		t.Fatalf("GetHuntStatus: %v", err)
	}
	if h.State != HuntStateStopped || h.ClientCount != 7 {
		t.Errorf("hunt = %+v", h)
	}
}

func TestGRPCClientGetHuntStatusNotFoundViaError(t *testing.T) {
	fake := &fakeHuntService{
		getHunt: func(ctx context.Context, in *veloapi.GetHuntRequest) (*veloapi.Hunt, error) {
			return nil, errors.New("rpc error: hunt not found: H.NOPE")
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	_, err := c.GetHuntStatus(context.Background(), "H.NOPE")
	if !errors.Is(err, ErrHuntNotFound) {
		t.Errorf("err = %v, want ErrHuntNotFound", err)
	}
}

func TestGRPCClientGetHuntStatusNotFoundViaEmptyResponse(t *testing.T) {
	fake := &fakeHuntService{
		getHunt: func(ctx context.Context, in *veloapi.GetHuntRequest) (*veloapi.Hunt, error) {
			return &veloapi.Hunt{}, nil
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	_, err := c.GetHuntStatus(context.Background(), "H.NOPE")
	if !errors.Is(err, ErrHuntNotFound) {
		t.Errorf("err = %v, want ErrHuntNotFound", err)
	}
}

func TestGRPCClientGetHuntResultsSuccess(t *testing.T) {
	huntSvc := &fakeHuntService{
		getHunt: func(ctx context.Context, in *veloapi.GetHuntRequest) (*veloapi.Hunt, error) {
			return &veloapi.Hunt{HuntId: "H.1", Artifacts: []string{"Windows.System.Pslist"}}, nil
		},
		getHuntResults: func(ctx context.Context, in *veloapi.GetHuntResultsRequest) (*veloapi.GetTableResponse, error) {
			if in.HuntId != "H.1" || in.Artifact != "Windows.System.Pslist" {
				t.Errorf("request = %+v", in)
			}
			return &veloapi.GetTableResponse{
				Columns:   []string{"Pid"},
				Rows:      []*veloapi.Row{jsonRow(t, 42)},
				TotalRows: 1,
			}, nil
		},
	}
	c := &grpcClient{hunts: huntSvc, timeout: time.Second, maxRows: 100}

	page, err := c.GetHuntResults(context.Background(), "H.1", "", 10, 1<<20)
	if err != nil {
		t.Fatalf("GetHuntResults: %v", err)
	}
	if len(page.Rows) != 1 {
		t.Errorf("Rows = %+v", page.Rows)
	}
}

func TestGRPCClientGetHuntResultsNotFound(t *testing.T) {
	huntSvc := &fakeHuntService{
		getHunt: func(ctx context.Context, in *veloapi.GetHuntRequest) (*veloapi.Hunt, error) {
			return &veloapi.Hunt{}, nil
		},
	}
	c := &grpcClient{hunts: huntSvc, timeout: time.Second, maxRows: 100}

	_, err := c.GetHuntResults(context.Background(), "H.NOPE", "", 10, 1<<20)
	if !errors.Is(err, ErrHuntNotFound) {
		t.Errorf("err = %v, want ErrHuntNotFound", err)
	}
}

// TestGRPCClientGetHuntResultsNamedSourceRequiresSelection is the hunt
// counterpart to TestGRPCClientGetFlowResultsNamedSourceRequiresSelection:
// a hunt collecting a multi-named-source artifact must report
// SourceRequired instead of silently returning zero rows. See
// docs/live-validation-report-v0.10.2.md finding 2.
func TestGRPCClientGetHuntResultsNamedSourceRequiresSelection(t *testing.T) {
	huntSvc := &fakeHuntService{
		getHunt: func(ctx context.Context, in *veloapi.GetHuntRequest) (*veloapi.Hunt, error) {
			return &veloapi.Hunt{HuntId: "H.1", Artifacts: []string{"Generic.Client.Info"}}, nil
		},
		getHuntResults: func(ctx context.Context, in *veloapi.GetHuntResultsRequest) (*veloapi.GetTableResponse, error) {
			t.Fatal("GetHuntResults RPC should not be called when a source must first be selected")
			return nil, nil
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
	c := &grpcClient{hunts: huntSvc, artifacts: artifacts, timeout: time.Second, maxRows: 100}

	page, err := c.GetHuntResults(context.Background(), "H.1", "", 10, 1<<20)
	if err != nil {
		t.Fatalf("GetHuntResults: %v", err)
	}
	if !page.SourceRequired {
		t.Fatal("SourceRequired = false, want true for a multi-source artifact with no source selected")
	}
	want := []string{"BasicInformation", "DetailedInfo", "LinuxInfo"}
	if len(page.AvailableSources) != len(want) {
		t.Fatalf("AvailableSources = %v, want %v", page.AvailableSources, want)
	}
}

// TestGRPCClientGetHuntResultsNamedSourceSelected proves the fix: an
// explicit source produces the source-qualified "Artifact/Source" name
// the real GetHuntResults RPC expects (confirmed against upstream's
// vql/server/hunts.HuntResultsPlugin.GetAvailableArtifacts, which builds
// its own valid-name list the identical way).
func TestGRPCClientGetHuntResultsNamedSourceSelected(t *testing.T) {
	huntSvc := &fakeHuntService{
		getHunt: func(ctx context.Context, in *veloapi.GetHuntRequest) (*veloapi.Hunt, error) {
			return &veloapi.Hunt{HuntId: "H.1", Artifacts: []string{"Generic.Client.Info"}}, nil
		},
		getHuntResults: func(ctx context.Context, in *veloapi.GetHuntResultsRequest) (*veloapi.GetTableResponse, error) {
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
	c := &grpcClient{hunts: huntSvc, artifacts: artifacts, timeout: time.Second, maxRows: 100}

	page, err := c.GetHuntResults(context.Background(), "H.1", "BasicInformation", 10, 1<<20)
	if err != nil {
		t.Fatalf("GetHuntResults: %v", err)
	}
	if page.SourceRequired {
		t.Fatal("SourceRequired = true, want false when an explicit source was given")
	}
	if len(page.Rows) != 1 || page.Rows[0]["Hostname"] != "mcp-lab-linux-01" {
		t.Errorf("Rows = %+v", page.Rows)
	}
}

// TestGRPCClientGetHuntResultsUnknownSourceRejected confirms an invalid
// caller-supplied source is rejected rather than silently querying a
// nonexistent table.
func TestGRPCClientGetHuntResultsUnknownSourceRejected(t *testing.T) {
	huntSvc := &fakeHuntService{
		getHunt: func(ctx context.Context, in *veloapi.GetHuntRequest) (*veloapi.Hunt, error) {
			return &veloapi.Hunt{HuntId: "H.1", Artifacts: []string{"Generic.Client.Info"}}, nil
		},
		getHuntResults: func(ctx context.Context, in *veloapi.GetHuntResultsRequest) (*veloapi.GetTableResponse, error) {
			t.Fatal("GetHuntResults RPC should not be called for an unknown source")
			return nil, nil
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
	c := &grpcClient{hunts: huntSvc, artifacts: artifacts, timeout: time.Second, maxRows: 100}

	_, err := c.GetHuntResults(context.Background(), "H.1", "NotARealSource", 10, 1<<20)
	if !errors.Is(err, ErrUnknownResultSource) {
		t.Errorf("err = %v, want wrapping ErrUnknownResultSource", err)
	}
}

func TestGRPCClientPreviewHuntScopeLabel(t *testing.T) {
	var sawCondition *veloapi.HuntCondition
	fake := &fakeHuntService{
		estimateHunt: func(ctx context.Context, in *veloapi.HuntEstimateRequest) (*veloapi.HuntStats, error) {
			sawCondition = in.Condition
			return &veloapi.HuntStats{TotalClientsScheduled: 12}, nil
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	preview, err := c.PreviewHuntScope(context.Background(), HuntScopeRequest{Label: "windows"})
	if err != nil {
		t.Fatalf("PreviewHuntScope: %v", err)
	}
	if preview.MatchedClientCount != 12 {
		t.Errorf("MatchedClientCount = %d, want 12", preview.MatchedClientCount)
	}
	labels := sawCondition.GetLabels()
	if labels == nil || len(labels.Label) != 1 || labels.Label[0] != "windows" {
		t.Errorf("condition = %+v, want a label condition for %q", sawCondition, "windows")
	}
}

func TestGRPCClientPreviewHuntScopeAll(t *testing.T) {
	var sawCondition *veloapi.HuntCondition
	fake := &fakeHuntService{
		estimateHunt: func(ctx context.Context, in *veloapi.HuntEstimateRequest) (*veloapi.HuntStats, error) {
			sawCondition = in.Condition
			return &veloapi.HuntStats{TotalClientsScheduled: 500}, nil
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	preview, err := c.PreviewHuntScope(context.Background(), HuntScopeRequest{All: true})
	if err != nil {
		t.Fatalf("PreviewHuntScope: %v", err)
	}
	if preview.MatchedClientCount != 500 {
		t.Errorf("MatchedClientCount = %d, want 500", preview.MatchedClientCount)
	}
	if sawCondition != nil {
		t.Errorf("condition = %+v, want nil (nil condition means all clients to EstimateHunt)", sawCondition)
	}
}

// TestGRPCClientPreviewHuntScopeClientIDsUnsupported and
// TestGRPCClientStartHuntClientIDsUnsupported together document the
// v0.9.0 blocker: Velociraptor's real HuntCondition proto has no
// explicit client-ID-list mode, so this project's client_ids hunt scope
// cannot be enacted against a real server. Both must fail closed
// (returning ErrHuntScopeClientIDsUnsupported) without ever calling
// EstimateHunt/CreateHunt, rather than silently starting an
// unrestricted or mistargeted hunt.
func TestGRPCClientPreviewHuntScopeClientIDsUnsupported(t *testing.T) {
	called := false
	fake := &fakeHuntService{
		estimateHunt: func(ctx context.Context, in *veloapi.HuntEstimateRequest) (*veloapi.HuntStats, error) {
			called = true
			return &veloapi.HuntStats{}, nil
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	_, err := c.PreviewHuntScope(context.Background(), HuntScopeRequest{ClientIDs: []string{"C.1234abcd5678ef90"}})
	if !errors.Is(err, ErrHuntScopeClientIDsUnsupported) {
		t.Errorf("err = %v, want ErrHuntScopeClientIDsUnsupported", err)
	}
	if called {
		t.Error("EstimateHunt was called for an unsupported client_ids scope")
	}
}

func TestGRPCClientStartHuntClientIDsUnsupported(t *testing.T) {
	called := false
	fake := &fakeHuntService{
		createHunt: func(ctx context.Context, in *veloapi.Hunt) (*veloapi.StartFlowResponse, error) {
			called = true
			return &veloapi.StartFlowResponse{}, nil
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	_, err := c.StartHunt(context.Background(), HuntRequest{
		Artifact: "Windows.System.Pslist",
		Scope:    HuntScopeRequest{ClientIDs: []string{"C.1234abcd5678ef90"}},
	})
	if !errors.Is(err, ErrHuntScopeClientIDsUnsupported) {
		t.Errorf("err = %v, want ErrHuntScopeClientIDsUnsupported", err)
	}
	if called {
		t.Error("CreateHunt was called for an unsupported client_ids scope")
	}
}

func TestGRPCClientStartHuntLabelScopeSuccess(t *testing.T) {
	var sawHunt *veloapi.Hunt
	fake := &fakeHuntService{
		createHunt: func(ctx context.Context, in *veloapi.Hunt) (*veloapi.StartFlowResponse, error) {
			sawHunt = in
			return &veloapi.StartFlowResponse{FlowId: "H.NEW1"}, nil
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	summary, err := c.StartHunt(context.Background(), HuntRequest{
		Artifact:   "Windows.System.Pslist",
		Parameters: map[string]string{"Pid": "1"},
		Scope:      HuntScopeRequest{Label: "windows"},
		MaxClients: 10,
	})
	if err != nil {
		t.Fatalf("StartHunt: %v", err)
	}
	if summary.HuntID != "H.NEW1" || summary.State != HuntStateRunning {
		t.Errorf("summary = %+v", summary)
	}
	if sawHunt.State != veloapi.Hunt_RUNNING {
		t.Errorf("State = %v, want RUNNING", sawHunt.State)
	}
	if sawHunt.ClientLimit != 10 {
		t.Errorf("ClientLimit = %d, want 10", sawHunt.ClientLimit)
	}
	if labels := sawHunt.GetCondition().GetLabels(); labels == nil || labels.Label[0] != "windows" {
		t.Errorf("condition = %+v", sawHunt.GetCondition())
	}
	if len(sawHunt.GetStartRequest().GetArtifacts()) != 1 || sawHunt.GetStartRequest().GetArtifacts()[0] != "Windows.System.Pslist" {
		t.Errorf("StartRequest.Artifacts = %v", sawHunt.GetStartRequest().GetArtifacts())
	}
}

func TestGRPCClientStartHuntError(t *testing.T) {
	fake := &fakeHuntService{
		createHunt: func(ctx context.Context, in *veloapi.Hunt) (*veloapi.StartFlowResponse, error) {
			return nil, errors.New("permission denied")
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	_, err := c.StartHunt(context.Background(), HuntRequest{Artifact: "X", Scope: HuntScopeRequest{All: true}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGRPCClientCancelHuntSuccess(t *testing.T) {
	var sawMutation *veloapi.HuntMutation
	fake := &fakeHuntService{
		modifyHunt: func(ctx context.Context, in *veloapi.HuntMutation) (*emptypb.Empty, error) {
			sawMutation = in
			return &emptypb.Empty{}, nil
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	if err := c.CancelHunt(context.Background(), "H.1"); err != nil {
		t.Fatalf("CancelHunt: %v", err)
	}
	if sawMutation.HuntId != "H.1" || sawMutation.State != veloapi.Hunt_STOPPED {
		t.Errorf("mutation = %+v", sawMutation)
	}
}

func TestGRPCClientCancelHuntError(t *testing.T) {
	fake := &fakeHuntService{
		modifyHunt: func(ctx context.Context, in *veloapi.HuntMutation) (*emptypb.Empty, error) {
			return nil, errors.New("permission denied")
		},
	}
	c := &grpcClient{hunts: fake, timeout: time.Second}

	if err := c.CancelHunt(context.Background(), "H.1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}
