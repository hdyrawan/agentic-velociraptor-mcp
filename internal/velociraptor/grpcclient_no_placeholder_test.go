package velociraptor

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor/veloapi"
)

// TestGRPCClientNoMethodReturnsErrNotImplemented is the v0.9.0 proof that
// every Client method a real, fully-wired grpcClient exposes actually
// calls through to a Velociraptor RPC rather than falling back to the
// embedded placeholderClient's ErrNotImplemented. Every fake sub-service
// below returns a minimal-but-valid response, so any method that still
// silently delegated to placeholderClient would fail this test with
// ErrNotImplemented instead of succeeding.
func TestGRPCClientNoMethodReturnsErrNotImplemented(t *testing.T) {
	c := &grpcClient{
		health: &fakeHealthChecker{resp: &veloapi.HealthCheckResponse{Status: veloapi.HealthCheckResponse_SERVING}},
		clients: &fakeClientSearcher{
			resp: &veloapi.SearchClientsResponse{Items: []*veloapi.ApiClient{{ClientId: "C.1234abcd5678ef90"}}},
		},
		clientDetail: &fakeClientGetter{resp: &veloapi.ApiClient{ClientId: "C.1234abcd5678ef90"}},
		artifacts:    &fakeArtifactCatalog{resp: &veloapi.ArtifactDescriptors{Items: []*veloapi.Artifact{{Name: "Generic.Client.Info"}}}},
		flows: &fakeFlowService{
			getClientFlows: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
				return &veloapi.GetTableResponse{Columns: []string{"FlowId"}, Rows: []*veloapi.Row{jsonRow(t, "F.1")}}, nil
			},
			getFlowDetails: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.FlowDetails, error) {
				return &veloapi.FlowDetails{Context: &veloapi.ArtifactCollectorContext{
					SessionId: "F.1",
					Request:   &veloapi.ArtifactCollectorArgs{Artifacts: []string{"Generic.Client.Info"}},
				}}, nil
			},
			collectArtifact: func(ctx context.Context, in *veloapi.ArtifactCollectorArgs) (*veloapi.ArtifactCollectorResponse, error) {
				return &veloapi.ArtifactCollectorResponse{FlowId: "F.NEW"}, nil
			},
			cancelFlow: func(ctx context.Context, in *veloapi.ApiFlowRequest) (*veloapi.StartFlowResponse, error) {
				return &veloapi.StartFlowResponse{FlowId: in.FlowId}, nil
			},
		},
		tables: &fakeTableService{
			getTable: func(ctx context.Context, in *veloapi.GetTableRequest) (*veloapi.GetTableResponse, error) {
				return &veloapi.GetTableResponse{
					Columns: []string{"Name", "Components", "Size"},
					Rows:    []*veloapi.Row{jsonRow(t, "memory.dmp", []string{"a"}, 10)},
				}, nil
			},
		},
		vfs: &fakeVFSService{
			vfsGetBuffer: func(ctx context.Context, in *veloapi.VFSFileBuffer) (*veloapi.VFSFileBuffer, error) {
				return &veloapi.VFSFileBuffer{Data: []byte("x")}, nil
			},
		},
		hunts: &fakeHuntService{
			createHunt: func(ctx context.Context, in *veloapi.Hunt) (*veloapi.StartFlowResponse, error) {
				return &veloapi.StartFlowResponse{FlowId: "H.1"}, nil
			},
			modifyHunt: func(ctx context.Context, in *veloapi.HuntMutation) (*emptypb.Empty, error) {
				return &emptypb.Empty{}, nil
			},
			listHunts: func(ctx context.Context, in *veloapi.ListHuntsRequest) (*veloapi.ListHuntsResponse, error) {
				return &veloapi.ListHuntsResponse{Items: []*veloapi.Hunt{{HuntId: "H.1"}}}, nil
			},
			getHunt: func(ctx context.Context, in *veloapi.GetHuntRequest) (*veloapi.Hunt, error) {
				return &veloapi.Hunt{HuntId: "H.1", Artifacts: []string{"Generic.Client.Info"}}, nil
			},
			getHuntResults: func(ctx context.Context, in *veloapi.GetHuntResultsRequest) (*veloapi.GetTableResponse, error) {
				return &veloapi.GetTableResponse{Columns: []string{"Pid"}}, nil
			},
			estimateHunt: func(ctx context.Context, in *veloapi.HuntEstimateRequest) (*veloapi.HuntStats, error) {
				return &veloapi.HuntStats{TotalClientsScheduled: 1}, nil
			},
		},
		timeout: time.Second,
		maxRows: 100,
	}

	ctx := context.Background()
	checks := map[string]error{}

	_, err := c.HealthCheck(ctx)
	checks["HealthCheck"] = err
	_, err = c.SearchClients(ctx, "", 10)
	checks["SearchClients"] = err
	_, err = c.GetClientInfo(ctx, "C.1234abcd5678ef90")
	checks["GetClientInfo"] = err
	_, err = c.ListArtifactNames(ctx)
	checks["ListArtifactNames"] = err
	_, err = c.GetArtifactDetails(ctx, "Generic.Client.Info")
	checks["GetArtifactDetails"] = err
	_, err = c.ListFlows(ctx, "C.1234abcd5678ef90", 10, "")
	checks["ListFlows"] = err
	_, err = c.GetFlowStatus(ctx, "C.1234abcd5678ef90", "F.1")
	checks["GetFlowStatus"] = err
	_, err = c.GetFlowResults(ctx, "C.1234abcd5678ef90", "F.1", "", 10, 1<<20, "")
	checks["GetFlowResults"] = err
	_, err = c.CollectArtifact(ctx, CollectionRequest{ClientID: "C.1234abcd5678ef90", Artifact: "Generic.Client.Info"})
	checks["CollectArtifact"] = err
	err = c.CancelFlow(ctx, "C.1234abcd5678ef90", "F.1")
	checks["CancelFlow"] = err
	_, err = c.ListFlowUploads(ctx, "C.1234abcd5678ef90", "F.1")
	checks["ListFlowUploads"] = err
	_, err = c.GetFlowUploadMetadata(ctx, "C.1234abcd5678ef90", "F.1", "memory.dmp")
	checks["GetFlowUploadMetadata"] = err
	_, err = c.DownloadFlowUpload(ctx, "C.1234abcd5678ef90", "F.1", "memory.dmp", 1<<20)
	checks["DownloadFlowUpload"] = err
	_, err = c.ListHunts(ctx, 10)
	checks["ListHunts"] = err
	_, err = c.GetHuntStatus(ctx, "H.1")
	checks["GetHuntStatus"] = err
	_, err = c.GetHuntResults(ctx, "H.1", "", 10, 1<<20)
	checks["GetHuntResults"] = err
	_, err = c.PreviewHuntScope(ctx, HuntScopeRequest{All: true})
	checks["PreviewHuntScope"] = err
	_, err = c.StartHunt(ctx, HuntRequest{Artifact: "Generic.Client.Info", Scope: HuntScopeRequest{All: true}})
	checks["StartHunt"] = err
	err = c.CancelHunt(ctx, "H.1")
	checks["CancelHunt"] = err

	for method, err := range checks {
		if errors.Is(err, ErrNotImplemented) {
			t.Errorf("%s: returned ErrNotImplemented — still falling through to placeholderClient", method)
		}
		if err != nil {
			t.Errorf("%s: unexpected error: %v", method, err)
		}
	}
}
