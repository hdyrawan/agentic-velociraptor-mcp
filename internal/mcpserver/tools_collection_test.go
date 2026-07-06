package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

const (
	testCollectClientID = "C.1234abcd5678ef90"
	testCollectArtifact = "Generic.Client.Info"
)

// --- velo_collect_artifact_with_approval ---

func TestCollectArtifactDisabledByDefault(t *testing.T) {
	deps, sink := testDeps(t) // Policy.Mode is read_only, Approvals is nil by default.
	handler := newCollectArtifactHandler(deps)

	_, out, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err == nil {
		t.Fatal("expected error when write pilot is disabled, got nil")
	}
	if out != (CollectArtifactOutput{}) {
		t.Errorf("expected zero-value output on early block, got %+v", out)
	}
	evt, ok := sink.last()
	if !ok {
		t.Fatal("expected an audit event")
	}
	if evt.Outcome != "blocked" {
		t.Errorf("audit Outcome = %q, want blocked", evt.Outcome)
	}
}

func TestCollectArtifactInvalidClientID(t *testing.T) {
	deps, _ := testWritePilotDeps(t)
	handler := newCollectArtifactHandler(deps)

	_, _, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: "not-a-client-id", Artifact: testCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err == nil {
		t.Fatal("expected error for invalid client id, got nil")
	}
}

func TestCollectArtifactInvalidArtifactSyntax(t *testing.T) {
	deps, _ := testWritePilotDeps(t)
	handler := newCollectArtifactHandler(deps)

	_, _, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: "not valid; DROP",
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err == nil {
		t.Fatal("expected error for invalid artifact syntax, got nil")
	}
}

func TestCollectArtifactNotAllowlisted(t *testing.T) {
	deps, _ := testWritePilotDeps(t)
	handler := newCollectArtifactHandler(deps)

	_, _, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: "Not.Allowlisted.Artifact",
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err == nil {
		t.Fatal("expected error for non-allowlisted artifact, got nil")
	}
}

func TestCollectArtifactMissingApproval(t *testing.T) {
	deps, sink := testWritePilotDeps(t)
	handler := newCollectArtifactHandler(deps)

	_, out, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "does-not-exist",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusNotFound {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusNotFound)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != "blocked" {
		t.Errorf("audit event = %+v, ok=%v, want Outcome=blocked", evt, ok)
	}
}

func TestCollectArtifactNotYetDecided(t *testing.T) {
	deps, _ := testWritePilotDeps(t)
	handler := newCollectArtifactHandler(deps)

	req := approval.Request{
		ID: "ref-1", Operation: approval.OperationCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst",
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
	}
	if _, err := deps.Approvals.Create(context.Background(), req); err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, out, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusError || !strings.Contains(out.Message, "not yet been decided") {
		t.Errorf("out = %+v, want error mentioning 'not yet been decided'", out)
	}
}

func TestCollectArtifactDenied(t *testing.T) {
	deps, _ := testWritePilotDeps(t)
	handler := newCollectArtifactHandler(deps)

	req := approval.Request{
		ID: "ref-1", Operation: approval.OperationCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst",
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
	}
	created, err := deps.Approvals.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := deps.Approvals.Decide(context.Background(), approval.Decision{RequestID: created.ID, Approved: false, ApprovedBy: "human", Note: "not justified"}); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	_, out, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusError || !strings.Contains(out.Message, "denied") {
		t.Errorf("out = %+v, want error mentioning denied", out)
	}
}

func TestCollectArtifactFingerprintMismatch(t *testing.T) {
	deps, _ := testWritePilotDeps(t)
	handler := newCollectArtifactHandler(deps)

	req := approval.Request{
		ID: "ref-1", Operation: approval.OperationCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst",
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
	}
	approveRequest(t, deps.Approvals, req)

	// Approved for Generic.Client.Info, but call requests a different
	// (still allowlisted) artifact.
	_, out, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: "Windows.System.Pslist",
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusError || !strings.Contains(out.Message, "does not match") {
		t.Errorf("out = %+v, want error mentioning mismatch", out)
	}
}

func TestCollectArtifactExpired(t *testing.T) {
	deps, _ := testWritePilotDeps(t)
	// Rebuild the approval store with a near-zero TTL to force expiry.
	store, err := approval.NewFileStore(deps.Config.Approval.StorePath+".expiry", time.Millisecond)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	deps.Approvals = store
	handler := newCollectArtifactHandler(deps)

	req := approval.Request{
		ID: "ref-1", Operation: approval.OperationCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst",
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
	}
	approveRequest(t, deps.Approvals, req)
	time.Sleep(5 * time.Millisecond)

	_, out, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusError || !strings.Contains(out.Message, "expired") {
		t.Errorf("out = %+v, want error mentioning expired", out)
	}
}

func TestCollectArtifactApprovedFakeExecutionSucceeds(t *testing.T) {
	deps, sink := testWritePilotDeps(t)
	deps.WriteClient = &fakeVelociraptorClient{
		collectArtifact: func(ctx context.Context, req velociraptor.CollectionRequest) (velociraptor.FlowSummary, error) {
			if req.ClientID != testCollectClientID || req.Artifact != testCollectArtifact {
				t.Fatalf("unexpected CollectArtifact call: %+v", req)
			}
			return velociraptor.FlowSummary{FlowID: "F.FAKE00000000", ClientID: req.ClientID, Artifact: req.Artifact, State: velociraptor.FlowStateRunning, CreatedAt: "2026-07-06T00:00:00Z"}, nil
		},
	}
	handler := newCollectArtifactHandler(deps)

	req := approval.Request{
		ID: "ref-1", Operation: approval.OperationCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst",
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
	}
	approveRequest(t, deps.Approvals, req)

	_, out, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Fatalf("Status = %q, want success: %+v", out.Status, out)
	}
	if out.FlowID != "F.FAKE00000000" {
		t.Errorf("FlowID = %q, want F.FAKE00000000", out.FlowID)
	}

	evt, ok := sink.last()
	if !ok {
		t.Fatal("expected an audit event")
	}
	if evt.Outcome != "success" || evt.ClientID != testCollectClientID || evt.Artifact != testCollectArtifact ||
		evt.CaseID != "CASE-1" || evt.RequestReason != "triage" || evt.ApprovalID != "ref-1" || evt.FlowID != "F.FAKE00000000" {
		t.Errorf("audit event = %+v, missing expected fields", evt)
	}

	// Approval is single-use: a second attempt against the same
	// reference must be blocked, not silently allowed to re-execute.
	_, out2, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error on reuse: %v", err)
	}
	if out2.Status != response.StatusError || !strings.Contains(out2.Message, "already been used") {
		t.Errorf("second attempt = %+v, want error mentioning already used", out2)
	}
}

func TestCollectArtifactWriteRPCFailureReportedHonestly(t *testing.T) {
	deps, sink := testWritePilotDeps(t)
	deps.WriteClient = &fakeVelociraptorClient{
		collectArtifact: func(ctx context.Context, req velociraptor.CollectionRequest) (velociraptor.FlowSummary, error) {
			return velociraptor.FlowSummary{}, velociraptor.ErrNotImplemented
		},
	}
	handler := newCollectArtifactHandler(deps)

	req := approval.Request{
		ID: "ref-1", Operation: approval.OperationCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst",
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
	}
	approveRequest(t, deps.Approvals, req)

	_, out, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID: testCollectClientID, Artifact: testCollectArtifact,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusError {
		t.Errorf("Status = %q, want error", out.Status)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != "error" {
		t.Errorf("audit event = %+v, ok=%v, want Outcome=error", evt, ok)
	}
}

// --- velo_collect_dfir_profile_with_approval ---

func TestCollectDFIRProfileApprovedFakeExecutionSucceeds(t *testing.T) {
	deps, _ := testWritePilotDeps(t)
	var collected []string
	deps.WriteClient = &fakeVelociraptorClient{
		collectArtifact: func(ctx context.Context, req velociraptor.CollectionRequest) (velociraptor.FlowSummary, error) {
			collected = append(collected, req.Artifact)
			return velociraptor.FlowSummary{FlowID: "F.FAKE" + req.Artifact, State: velociraptor.FlowStateRunning}, nil
		},
	}
	handler := newCollectDFIRProfileHandler(deps)

	const profileName = "windows_basic_triage"
	req := approval.Request{
		ID: "ref-1", Operation: approval.OperationCollectDFIRProfile,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst",
		ClientID: testCollectClientID, Profile: profileName,
	}
	approveRequest(t, deps.Approvals, req)

	_, out, err := handler(context.Background(), nil, CollectDFIRProfileInput{
		ClientID: testCollectClientID, Profile: profileName,
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Fatalf("Status = %q, want success: %+v", out.Status, out)
	}
	if len(out.Flows) == 0 || len(collected) != len(out.Flows) {
		t.Errorf("Flows = %+v, collected = %v, want matching non-empty sets", out.Flows, collected)
	}
}

func TestCollectDFIRProfileNotAllowlisted(t *testing.T) {
	deps, _ := testWritePilotDeps(t)
	handler := newCollectDFIRProfileHandler(deps)

	// not_a_real_profile parses as a syntactically valid profile name
	// but is neither allowlisted nor loaded.
	_, out, err := handler(context.Background(), nil, CollectDFIRProfileInput{
		ClientID: testCollectClientID, Profile: "not_a_real_profile",
		CaseID: "CASE-1", Reason: "triage", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err == nil {
		t.Fatalf("expected error for non-allowlisted/unknown profile, got out=%+v", out)
	}
}

// --- velo_cancel_flow_with_approval ---

func TestCancelFlowInvalidFlowID(t *testing.T) {
	deps, _ := testWritePilotDeps(t)
	handler := newCancelFlowHandler(deps)

	_, _, err := handler(context.Background(), nil, CancelFlowInput{
		ClientID: testCollectClientID, FlowID: "not-a-flow-id",
		CaseID: "CASE-1", Reason: "stop it", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err == nil {
		t.Fatal("expected error for invalid flow id, got nil")
	}
}

func TestCancelFlowApprovedFakeExecutionSucceeds(t *testing.T) {
	deps, sink := testWritePilotDeps(t)
	var cancelledClient, cancelledFlow string
	deps.WriteClient = &fakeVelociraptorClient{
		cancelFlow: func(ctx context.Context, clientID, flowID string) error {
			cancelledClient, cancelledFlow = clientID, flowID
			return nil
		},
	}
	handler := newCancelFlowHandler(deps)

	const flowID = "F.BN2HJC4N4T6KG"
	req := approval.Request{
		ID: "ref-1", Operation: approval.OperationCancelFlow,
		CaseID: "CASE-1", Reason: "stop it", Requester: "analyst",
		ClientID: testCollectClientID, FlowID: flowID,
	}
	approveRequest(t, deps.Approvals, req)

	_, out, err := handler(context.Background(), nil, CancelFlowInput{
		ClientID: testCollectClientID, FlowID: flowID,
		CaseID: "CASE-1", Reason: "stop it", Requester: "analyst", ApprovalReference: "ref-1",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Fatalf("Status = %q, want success: %+v", out.Status, out)
	}
	if cancelledClient != testCollectClientID || cancelledFlow != flowID {
		t.Errorf("CancelFlow called with (%q, %q), want (%q, %q)", cancelledClient, cancelledFlow, testCollectClientID, flowID)
	}
	if evt, ok := sink.last(); !ok || evt.Outcome != "success" || evt.FlowID != flowID {
		t.Errorf("audit event = %+v, ok=%v", evt, ok)
	}
}
