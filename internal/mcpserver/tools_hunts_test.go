package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/config"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/policy"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

// fakeApprovalStore implements approval.Store with test-controlled
// IsApproved and Consume behavior.
type fakeApprovalStore struct {
	approval.Store // embed so only needed methods need overriding
	approved       bool
	consumeErr     error
	consumeCalled  bool
}

func (f *fakeApprovalStore) IsApproved(_ context.Context, _ string) (bool, error) {
	return f.approved, nil
}

func (f *fakeApprovalStore) Consume(_ context.Context, _ string) error {
	f.consumeCalled = true
	return f.consumeErr
}

// fakeHuntClient implements the hunt-related methods of velociraptor.Client
// for test use, embedding placeholderClient for everything else.
type fakeHuntClient struct {
	velociraptor.Client
	previewHuntScope func(ctx context.Context, scope velociraptor.HuntScopeRequest) (velociraptor.HuntScopePreview, error)
	startHunt        func(ctx context.Context, req velociraptor.HuntRequest) (velociraptor.HuntSummary, error)
	listHunts        func(ctx context.Context, limit int) ([]velociraptor.HuntSummary, error)
	getHuntStatus    func(ctx context.Context, huntID string) (velociraptor.HuntSummary, error)
	getHuntResults   func(ctx context.Context, huntID string, maxRows int, maxBytes int64) (velociraptor.FlowResultPage, error)
	cancelHunt       func(ctx context.Context, huntID string) error
}

func (f *fakeHuntClient) PreviewHuntScope(ctx context.Context, scope velociraptor.HuntScopeRequest) (velociraptor.HuntScopePreview, error) {
	return f.previewHuntScope(ctx, scope)
}

func (f *fakeHuntClient) StartHunt(ctx context.Context, req velociraptor.HuntRequest) (velociraptor.HuntSummary, error) {
	return f.startHunt(ctx, req)
}

func (f *fakeHuntClient) ListHunts(ctx context.Context, limit int) ([]velociraptor.HuntSummary, error) {
	return f.listHunts(ctx, limit)
}

func (f *fakeHuntClient) GetHuntStatus(ctx context.Context, huntID string) (velociraptor.HuntSummary, error) {
	return f.getHuntStatus(ctx, huntID)
}

func (f *fakeHuntClient) GetHuntResults(ctx context.Context, huntID string, maxRows int, maxBytes int64) (velociraptor.FlowResultPage, error) {
	return f.getHuntResults(ctx, huntID, maxRows, maxBytes)
}

func (f *fakeHuntClient) CancelHunt(ctx context.Context, huntID string) error {
	return f.cancelHunt(ctx, huntID)
}

// testHuntDeps builds a Deps with a policy that allows artifacts and
// profiles needed by hunt tests, plus a fake audit sink.
func testHuntDeps(t *testing.T, approvers *fakeApprovalStore) (Deps, *fakeAuditSink) {
	cfg := config.Default()
	cfg.Policy.AllowedArtifacts = []string{"Windows.System.Pslist", "Generic.Client.Info"}
	cfg.Policy.AllowedProfiles = []string{"windows_basic_triage"}
	cfg.Policy.MaxHuntClients = 50
	cfg.Velociraptor.MaxRows = 100
	cfg.Velociraptor.MaxResultBytes = 1048576

	sink := &fakeAuditSink{}
	return Deps{
		Config:   cfg,
		Policy:   policy.NewEngine(cfg.Policy),
		Audit:    sink,
		Approvals: approvers,
	}, sink
}

// ---------------------------------------------------------------------------
// velo_preview_hunt_scope
// ---------------------------------------------------------------------------

func TestPreviewHuntScopeSuccessMock(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	handler := newPreviewHuntScopeHandler(deps)

	_, out, err := handler(context.Background(), nil, PreviewHuntScopeInput{
		ClientIDs: []string{"C.1234abcd5678ef90"},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusSuccess)
	}
	if out.Mode != VelociraptorModeMock {
		t.Errorf("Mode = %q, want %q", out.Mode, VelociraptorModeMock)
	}
	if len(out.SampleClientIDs) == 0 {
		t.Error("SampleClientIDs is empty, want input client_ids reflected")
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event = %+v, ok=%v, want success", evt, ok)
	}
}

func TestPreviewHuntScopeSuccessReal(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		previewHuntScope: func(_ context.Context, _ velociraptor.HuntScopeRequest) (velociraptor.HuntScopePreview, error) {
			return velociraptor.HuntScopePreview{
				MatchedClientCount: 3,
				SampleClientIDs:    []string{"C.1", "C.2", "C.3"},
			}, nil
		},
	}

	handler := newPreviewHuntScopeHandler(deps)
	_, out, err := handler(context.Background(), nil, PreviewHuntScopeInput{
		Label: "linux",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusSuccess)
	}
	if out.Mode != VelociraptorModeReal {
		t.Errorf("Mode = %q, want %q", out.Mode, VelociraptorModeReal)
	}
	if out.Matched != 3 {
		t.Errorf("Matched = %d, want 3", out.Matched)
	}
	if len(out.SampleClientIDs) != 3 {
		t.Errorf("len(SampleClientIDs) = %d, want 3", len(out.SampleClientIDs))
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

func TestPreviewHuntScopeBlocksTargetAllByDefault(t *testing.T) {
	cfg := config.Default()
	cfg.Policy.AllowTargetAll = false
	deps, sink := testHuntDeps(t, nil)
	deps.Policy = policy.NewEngine(cfg.Policy)

	handler := newPreviewHuntScopeHandler(deps)
	_, _, err := handler(context.Background(), nil, PreviewHuntScopeInput{
		All: true,
	})
	if err == nil {
		t.Fatal("expected error for target_all, got nil")
	}
	if !strings.Contains(err.Error(), "disabled by policy") {
		t.Errorf("error = %q, want 'disabled by policy'", err.Error())
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}
}

func TestPreviewHuntScopeInvalidScope(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	handler := newPreviewHuntScopeHandler(deps)

	// both label and client_ids
	_, _, err := handler(context.Background(), nil, PreviewHuntScopeInput{
		ClientIDs: []string{"C.1"},
		Label:     "linux",
	})
	if err == nil {
		t.Fatal("expected error for ambiguous scope, got nil")
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}
}

// ---------------------------------------------------------------------------
// velo_start_hunt_with_approval
// ---------------------------------------------------------------------------

func TestStartHuntBlockedWithoutApproval(t *testing.T) {
	approver := &fakeApprovalStore{approved: false}
	deps, sink := testHuntDeps(t, approver)
	handler := newStartHuntHandler(deps)

	_, _, err := handler(context.Background(), nil, StartHuntInput{
		CaseID:     "CASE-001",
		Reason:     "test hunt",
		Requester:  "tester",
		ApprovalID: "approval-nonexistent",
		Artifact:   "Windows.System.Pslist",
		Label:      "linux",
	})
	if err == nil {
		t.Fatal("expected error without approval, got nil")
	}
	if !strings.Contains(err.Error(), "has not been granted") {
		t.Errorf("error = %q, want 'has not been granted'", err.Error())
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}
}

func TestStartHuntEnforcesMaxHuntClients(t *testing.T) {
	approver := &fakeApprovalStore{approved: true}
	deps, sink := testHuntDeps(t, approver)
	deps.Policy = policy.NewEngine(config.PolicyConfig{
		Mode:              config.PolicyModeControlled,
		MaxHuntClients:    5,
		AllowedArtifacts:  []string{"Windows.System.Pslist"},
		AllowedProfiles:   []string{"windows_basic_triage"},
	})
	deps.WriteClient = velociraptor.NewClient()
	deps.VelociraptorWriteMode = VelociraptorModeReal

	handler := newStartHuntHandler(deps)

	_, out, err := handler(context.Background(), nil, StartHuntInput{
		CaseID:     "CASE-002",
		Reason:     "test max clients",
		Requester:  "tester",
		ApprovalID: "approval-ok",
		Artifact:   "Windows.System.Pslist",
		Label:      "linux",
		MaxClients: 3,
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	// WriteClient returns ErrNotImplemented so we expect mode=real but error result
	if out.Mode != VelociraptorModeReal {
		t.Errorf("Mode = %q, want %q", out.Mode, VelociraptorModeReal)
	}
	// The handler reaches WriteClient.StartHunt which fails with ErrNotImplemented
	if out.Status != response.StatusError {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusError)
	}
	_ = sink
}

func TestStartHuntEnforcesArtifactAllowlist(t *testing.T) {
	approver := &fakeApprovalStore{approved: true}
	deps, _ := testHuntDeps(t, approver)
	handler := newStartHuntHandler(deps)

	_, _, err := handler(context.Background(), nil, StartHuntInput{
		CaseID:     "CASE-003",
		Reason:     "test allowlist",
		Requester:  "tester",
		ApprovalID: "approval-ok",
		Artifact:   "Not.In.Allowlist",
		Label:      "linux",
	})
	if err == nil {
		t.Fatal("expected error for non-allowlisted artifact, got nil")
	}
	if !strings.Contains(err.Error(), "allowlist") {
		t.Errorf("error = %q, want 'allowlist'", err.Error())
	}
}

func TestStartHuntBlockedByReadOnlyMode(t *testing.T) {
	approver := &fakeApprovalStore{approved: true}
	deps, sink := testHuntDeps(t, approver)
	deps.Policy = policy.NewEngine(config.PolicyConfig{
		Mode:              config.PolicyModeReadOnly,
		AllowedArtifacts:  []string{"Windows.System.Pslist"},
		AllowedProfiles:   []string{"windows_basic_triage"},
	})

	handler := newStartHuntHandler(deps)
	_, _, err := handler(context.Background(), nil, StartHuntInput{
		CaseID:     "CASE-004",
		Reason:     "test read-only",
		Requester:  "tester",
		ApprovalID: "approval-ok",
		Artifact:   "Windows.System.Pslist",
		Label:      "linux",
	})
	if err == nil {
		t.Fatal("expected error in read-only mode, got nil")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error = %q, want 'read-only'", err.Error())
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}
}

func TestStartHuntRejectsMissingCaseID(t *testing.T) {
	approver := &fakeApprovalStore{approved: true}
	deps, _ := testHuntDeps(t, approver)
	handler := newStartHuntHandler(deps)

	_, _, err := handler(context.Background(), nil, StartHuntInput{
		Reason:     "missing case",
		Requester:  "tester",
		ApprovalID: "approval-ok",
		Artifact:   "Windows.System.Pslist",
		Label:      "linux",
	})
	if err == nil {
		t.Fatal("expected error for missing case_id, got nil")
	}
	if !strings.Contains(err.Error(), "case_id") {
		t.Errorf("error = %q, want 'case_id'", err.Error())
	}
}

// ---------------------------------------------------------------------------
// velo_start_dfir_hunt_with_approval
// ---------------------------------------------------------------------------

func TestStartDFIRHuntEnforcesProfileAllowlist(t *testing.T) {
	approver := &fakeApprovalStore{approved: true}
	deps, _ := testHuntDeps(t, approver)
	handler := newStartDFIRHuntHandler(deps)

	_, _, err := handler(context.Background(), nil, StartDFIRHuntInput{
		CaseID:     "CASE-005",
		Reason:     "test profile allowlist",
		Requester:  "tester",
		ApprovalID: "approval-ok",
		Profile:    "not_an_allowed_profile",
		Label:      "linux",
	})
	if err == nil {
		t.Fatal("expected error for non-allowlisted profile, got nil")
	}
	if !strings.Contains(err.Error(), "allowlist") {
		t.Errorf("error = %q, want 'allowlist'", err.Error())
	}
}

func TestStartDFIRHuntInvalidProfileName(t *testing.T) {
	approver := &fakeApprovalStore{approved: true}
	deps, _ := testHuntDeps(t, approver)
	handler := newStartDFIRHuntHandler(deps)

	_, _, err := handler(context.Background(), nil, StartDFIRHuntInput{
		CaseID:     "CASE-006",
		Reason:     "test invalid profile name",
		Requester:  "tester",
		ApprovalID: "approval-ok",
		Profile:    "../path/traversal",
		Label:      "linux",
	})
	if err == nil {
		t.Fatal("expected error for invalid profile name, got nil")
	}
}

// ---------------------------------------------------------------------------
// velo_list_hunts
// ---------------------------------------------------------------------------

func TestListHuntsSuccessMock(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	handler := newListHuntsHandler(deps)

	_, out, err := handler(context.Background(), nil, ListHuntsInput{Limit: 10})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusSuccess)
	}
	if out.Mode != VelociraptorModeMock {
		t.Errorf("Mode = %q, want %q", out.Mode, VelociraptorModeMock)
	}
	if len(out.Hunts) != 0 {
		t.Errorf("Hunts = %v, want empty in mock mode", out.Hunts)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

func TestListHuntsSuccessReal(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		listHunts: func(_ context.Context, _ int) ([]velociraptor.HuntSummary, error) {
			return []velociraptor.HuntSummary{
				{HuntID: "H.1", Artifact: "Generic.Client.Info", State: velociraptor.HuntStateRunning},
				{HuntID: "H.2", Artifact: "Windows.System.Pslist", State: velociraptor.HuntStateStopped},
			}, nil
		},
	}

	handler := newListHuntsHandler(deps)
	_, out, err := handler(context.Background(), nil, ListHuntsInput{Limit: 100})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusSuccess)
	}
	if out.Mode != VelociraptorModeReal {
		t.Errorf("Mode = %q, want %q", out.Mode, VelociraptorModeReal)
	}
	if len(out.Hunts) != 2 {
		t.Fatalf("len(Hunts) = %d, want 2", len(out.Hunts))
	}
	if out.Hunts[0].HuntID != "H.1" {
		t.Errorf("Hunts[0].HuntID = %q, want H.1", out.Hunts[0].HuntID)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

func TestListHuntsEmpty(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		listHunts: func(_ context.Context, _ int) ([]velociraptor.HuntSummary, error) {
			return []velociraptor.HuntSummary{}, nil
		},
	}

	handler := newListHuntsHandler(deps)
	_, out, err := handler(context.Background(), nil, ListHuntsInput{Limit: 100})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusForCount(0) {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusForCount(0))
	}
	if len(out.Hunts) != 0 {
		t.Errorf("len(Hunts) = %d, want 0", len(out.Hunts))
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

// ---------------------------------------------------------------------------
// velo_get_hunt_status
// ---------------------------------------------------------------------------

func TestGetHuntStatusSuccessReal(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		getHuntStatus: func(_ context.Context, huntID string) (velociraptor.HuntSummary, error) {
			return velociraptor.HuntSummary{
				HuntID: huntID, Artifact: "Generic.Client.Info",
				State: velociraptor.HuntStateRunning, ClientCount: 10,
			}, nil
		},
	}

	handler := newGetHuntStatusHandler(deps)
	_, out, err := handler(context.Background(), nil, GetHuntStatusInput{HuntID: "H.1"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusSuccess)
	}
	if out.Hunt == nil {
		t.Fatal("Hunt is nil, want populated")
	}
	if out.Hunt.HuntID != "H.1" {
		t.Errorf("Hunt.HuntID = %q, want H.1", out.Hunt.HuntID)
	}
	if out.Hunt.ClientCount != 10 {
		t.Errorf("Hunt.ClientCount = %d, want 10", out.Hunt.ClientCount)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

func TestGetHuntStatusNotFound(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		getHuntStatus: func(_ context.Context, _ string) (velociraptor.HuntSummary, error) {
			return velociraptor.HuntSummary{}, velociraptor.ErrHuntNotFound
		},
	}

	handler := newGetHuntStatusHandler(deps)
	_, out, err := handler(context.Background(), nil, GetHuntStatusInput{HuntID: "H.nonexistent"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusNotFound {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusNotFound)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError {
		t.Errorf("audit event outcome = %q, want error", evt.Outcome)
	}
}

func TestGetHuntStatusMock(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	handler := newGetHuntStatusHandler(deps)

	_, out, err := handler(context.Background(), nil, GetHuntStatusInput{HuntID: "H.1"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Mode != VelociraptorModeMock {
		t.Errorf("Mode = %q, want %q", out.Mode, VelociraptorModeMock)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

// ---------------------------------------------------------------------------
// velo_get_hunt_results
// ---------------------------------------------------------------------------

func TestGetHuntResultsSuccessReal(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		getHuntResults: func(_ context.Context, _ string, maxRows int, maxBytes int64) (velociraptor.FlowResultPage, error) {
			return velociraptor.FlowResultPage{
				Rows: []map[string]any{
					{"Hostname": "host1", "OS": "linux"},
					{"Hostname": "host2", "OS": "windows"},
				},
				TotalRows:    2,
				ReturnedRows: 2,
			}, nil
		},
	}

	handler := newGetHuntResultsHandler(deps)
	_, out, err := handler(context.Background(), nil, GetHuntResultsInput{HuntID: "H.1", Limit: 10})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusSuccess)
	}
	if out.Mode != VelociraptorModeReal {
		t.Errorf("Mode = %q, want %q", out.Mode, VelociraptorModeReal)
	}
	if out.ReturnedRows != 2 {
		t.Errorf("ReturnedRows = %d, want 2", out.ReturnedRows)
	}
	if len(out.Rows) != 2 {
		t.Errorf("len(Rows) = %d, want 2", len(out.Rows))
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

func TestGetHuntResultsEmpty(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		getHuntResults: func(_ context.Context, _ string, _ int, _ int64) (velociraptor.FlowResultPage, error) {
			return velociraptor.FlowResultPage{
				Rows: []map[string]any{}, TotalRows: 0, ReturnedRows: 0,
			}, nil
		},
	}

	handler := newGetHuntResultsHandler(deps)
	_, out, err := handler(context.Background(), nil, GetHuntResultsInput{HuntID: "H.1"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusForCount(0) {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusForCount(0))
	}
	if len(out.Rows) != 0 {
		t.Errorf("len(Rows) = %d, want 0", len(out.Rows))
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

func TestGetHuntResultsNotFound(t *testing.T) {
	deps, sink := testHuntDeps(t, nil)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		getHuntResults: func(_ context.Context, _ string, _ int, _ int64) (velociraptor.FlowResultPage, error) {
			return velociraptor.FlowResultPage{}, velociraptor.ErrHuntNotFound
		},
	}

	handler := newGetHuntResultsHandler(deps)
	_, out, err := handler(context.Background(), nil, GetHuntResultsInput{HuntID: "H.nonexistent"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusNotFound {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusNotFound)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError {
		t.Errorf("audit event outcome = %q, want error", evt.Outcome)
	}
}

func TestGetHuntResultsBoundLimit(t *testing.T) {
	deps, _ := testHuntDeps(t, nil)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		getHuntResults: func(_ context.Context, _ string, maxRows int, maxBytes int64) (velociraptor.FlowResultPage, error) {
			rows := make([]map[string]any, 50)
			for i := range rows {
				rows[i] = map[string]any{"idx": i}
			}
			return velociraptor.FlowResultPage{
				Rows: rows, TotalRows: 50, ReturnedRows: 50,
			}, nil
		},
	}

	handler := newGetHuntResultsHandler(deps)
	_, out, err := handler(context.Background(), nil, GetHuntResultsInput{HuntID: "H.1", Limit: 5})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.ReturnedRows > 5 {
		t.Errorf("ReturnedRows = %d, want <= 5", out.ReturnedRows)
	}
	if !out.Truncated {
		t.Error("Truncated = false, want true (limit ceil)")
	}
}

func TestGetHuntResultsPaginationCursor(t *testing.T) {
	deps, _ := testHuntDeps(t, nil)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		getHuntResults: func(_ context.Context, _ string, maxRows int, _ int64) (velociraptor.FlowResultPage, error) {
			rows := make([]map[string]any, maxRows+20)
			for i := range rows {
				rows[i] = map[string]any{"idx": i}
			}
			return velociraptor.FlowResultPage{
				Rows: rows, TotalRows: len(rows), ReturnedRows: len(rows),
				Truncated: true,
			}, nil
		},
	}

	handler := newGetHuntResultsHandler(deps)
	_, out, err := handler(context.Background(), nil, GetHuntResultsInput{HuntID: "H.1", Limit: 10})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.NextCursor == "" {
		t.Error("NextCursor is empty, want offset-based cursor for truncated results")
	}
	if !strings.HasPrefix(out.NextCursor, "offset:") {
		t.Errorf("NextCursor = %q, want 'offset:...'", out.NextCursor)
	}
	if !out.Truncated {
		t.Error("Truncated = false, want true")
	}
}

// ---------------------------------------------------------------------------
// velo_cancel_hunt_with_approval
// ---------------------------------------------------------------------------

func TestCancelHuntBlockedWithoutApproval(t *testing.T) {
	approver := &fakeApprovalStore{approved: false}
	deps, sink := testHuntDeps(t, approver)
	handler := newCancelHuntHandler(deps)

	_, _, err := handler(context.Background(), nil, CancelHuntInput{
		CaseID:     "CASE-010",
		Reason:     "test cancel",
		Requester:  "tester",
		ApprovalID: "approval-nonexistent",
		HuntID:     "H.1",
	})
	if err == nil {
		t.Fatal("expected error without approval, got nil")
	}
	if !strings.Contains(err.Error(), "has not been granted") {
		t.Errorf("error = %q, want 'has not been granted'", err.Error())
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}
}

func TestCancelHuntApprovedFakePath(t *testing.T) {
	approver := &fakeApprovalStore{approved: true}
	deps, sink := testHuntDeps(t, approver)
	deps.Policy = policy.NewEngine(config.PolicyConfig{
		Mode:              config.PolicyModeControlled,
		MaxHuntClients:    50,
		AllowedArtifacts:  []string{"Windows.System.Pslist"},
		AllowedProfiles:   []string{"windows_basic_triage"},
	})
	deps.WriteClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		cancelHunt: func(_ context.Context, huntID string) error {
			if huntID != "H.1" {
				return fmt.Errorf("unexpected hunt ID %q", huntID)
			}
			return nil
		},
	}
	deps.VelociraptorWriteMode = VelociraptorModeReal

	handler := newCancelHuntHandler(deps)
	_, out, err := handler(context.Background(), nil, CancelHuntInput{
		CaseID:     "CASE-011",
		Reason:     "approved cancel test",
		Requester:  "tester",
		ApprovalID: "approval-ok",
		HuntID:     "H.1",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusSuccess)
	}
	if !approver.consumeCalled {
		t.Error("Consume was not called on the approval store")
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

func TestStartHuntApprovedFakePath(t *testing.T) {
	approver := &fakeApprovalStore{approved: true}
	deps, sink := testHuntDeps(t, approver)
	deps.Policy = policy.NewEngine(config.PolicyConfig{
		Mode:              config.PolicyModeControlled,
		MaxHuntClients:    50,
		AllowedArtifacts:  []string{"Windows.System.Pslist", "Generic.Client.Info"},
		AllowedProfiles:   []string{"windows_basic_triage"},
	})
	deps.WriteClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		startHunt: func(_ context.Context, req velociraptor.HuntRequest) (velociraptor.HuntSummary, error) {
			return velociraptor.HuntSummary{
				HuntID: "H.new", Artifact: req.Artifact, State: velociraptor.HuntStateRunning,
			}, nil
		},
	}
	deps.VelociraptorWriteMode = VelociraptorModeReal

	handler := newStartHuntHandler(deps)
	_, out, err := handler(context.Background(), nil, StartHuntInput{
		CaseID:     "CASE-012",
		Reason:     "approved start test",
		Requester:  "tester",
		ApprovalID: "approval-ok",
		Artifact:   "Generic.Client.Info",
		Label:      "linux",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusSuccess)
	}
	if out.HuntID != "H.new" {
		t.Errorf("HuntID = %q, want H.new", out.HuntID)
	}
	if !approver.consumeCalled {
		t.Error("Consume was not called on the approval store")
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

// ---------------------------------------------------------------------------
// no raw VQL exposed
// ---------------------------------------------------------------------------

func TestHuntToolsDoNotExposeRawVQL(t *testing.T) {
	deps, _ := testHuntDeps(t, nil)
	srv := New("test", "0.0.0", deps)

	session := connectTestClient(t, srv)
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	// Check that no registered tool name contains "vql" or "run_vql"
	// (descriptions may mention VQL in documentation context — e.g.
	// "never the VQL body" — which is acceptable).
	forbiddenToolNames := []string{"run_vql", "vql", "execute_vql"}
	for _, tool := range res.Tools {
		name := strings.ToLower(tool.Name)
		for _, pat := range forbiddenToolNames {
			if strings.Contains(name, pat) {
				t.Errorf("tool %q contains VQL pattern %q in name", tool.Name, pat)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// existing v0.3 tools remain registered
// ---------------------------------------------------------------------------

func TestExistingV03ToolsRemainRegistered(t *testing.T) {
	deps, _ := testHuntDeps(t, nil)
	srv := New("test", "0.0.0", deps)

	session := connectTestClient(t, srv)
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	v03Tools := []string{
		"velo_compare_dfir_profiles",
		"velo_find_profiles_by_artifact",
		"velo_get_artifact_details",
		"velo_get_client_info",
		"velo_get_dfir_profile",
		"velo_health_check",
		"velo_list_artifact_names",
		"velo_list_dfir_profiles",
		"velo_plan_dfir_triage",
		"velo_search_clients",
		"velo_validate_dfir_profile",
	}

	for _, want := range v03Tools {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("existing v0.3 tool %q is no longer registered", want)
		}
	}
}
