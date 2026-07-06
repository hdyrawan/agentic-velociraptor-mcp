package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

// failingSink simulates a broken audit log (disk full, deleted file,
// permission change): every write fails.
type failingSink struct{}

func (failingSink) Write(audit.Event) error { return errors.New("disk full") }
func (failingSink) Close() error            { return nil }

// ---------------------------------------------------------------------------
// Q1: CLI-created hunt_ioc approvals verify through the handler
// ---------------------------------------------------------------------------

// TestCLIBuiltHuntIOCApprovalVerifiesThroughHandler is the regression
// test for the "hunt_ioc has no CLI approval path" finding: it builds an
// approval.Request exactly the way the `approve` CLI subcommand now does
// (BuildHuntIOCApprovalRequest, the same validation/template-binding
// path the handler uses), stores and approves it, then drives
// velo_hunt_ioc_with_approval with the same inputs and requires the
// fingerprint check to pass, the hunt to start, and the approval to be
// consumed.
func TestCLIBuiltHuntIOCApprovalVerifiesThroughHandler(t *testing.T) {
	deps, _, store := testIOCDeps(t)

	var startedArtifact string
	var startedParams map[string]string
	deps.WriteClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		startHunt: func(_ context.Context, req velociraptor.HuntRequest) (velociraptor.HuntSummary, error) {
			startedArtifact = req.Artifact
			startedParams = req.Parameters
			return velociraptor.HuntSummary{HuntID: "H.cli-parity", Artifact: req.Artifact, State: velociraptor.HuntStateRunning}, nil
		},
	}
	deps.VelociraptorWriteMode = VelociraptorModeReal

	// Exactly what runApprove does for --operation hunt_ioc.
	built, err := BuildHuntIOCApprovalRequest(
		"CASE-CLI-1", "hunt this hash", "analyst@example.com",
		"hash", "d41d8cd98f00b204e9800998ecf8427e",
		nil, "windows", false,
	)
	if err != nil {
		t.Fatalf("BuildHuntIOCApprovalRequest: %v", err)
	}
	built.ID = "CASE-CLI-1-REF"
	if _, err := store.Create(context.Background(), built); err != nil {
		t.Fatalf("store.Create: %v", err)
	}
	if err := store.Decide(context.Background(), approval.Decision{
		RequestID: built.ID, Approved: true, ApprovedBy: "ir-lead@example.com",
	}); err != nil {
		t.Fatalf("store.Decide: %v", err)
	}

	handler := newHuntIOCHandler(deps)
	_, out, err := handler(context.Background(), nil, HuntIOCInput{
		CaseID:     "CASE-CLI-1",
		Reason:     "hunt this hash",
		Requester:  "analyst@example.com",
		ApprovalID: built.ID,
		Kind:       "hash",
		Value:      "d41d8cd98f00b204e9800998ecf8427e",
		Label:      "windows",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Fatalf("Status = %q (%s), want success: CLI-built approval must fingerprint-match the handler candidate", out.Status, out.Message)
	}
	if startedArtifact != "System.Hash.Hunt" {
		t.Errorf("StartHunt artifact = %q, want System.Hash.Hunt", startedArtifact)
	}
	if startedParams["HashValue"] != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("StartHunt params = %v, want HashValue bound to the approved hash", startedParams)
	}

	status, err := store.Get(context.Background(), built.ID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if !status.Consumed {
		t.Error("approval was not consumed by the successful execution")
	}
}

// ---------------------------------------------------------------------------
// S2: hunt/IOC write-path field validation
// ---------------------------------------------------------------------------

func TestStartHuntRejectsBadWriteFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(in *StartHuntInput)
		want   string
	}{
		{"empty case_id", func(in *StartHuntInput) { in.CaseID = "" }, "case_id"},
		{"newline in case_id", func(in *StartHuntInput) { in.CaseID = "CASE-1\nclient=C.x" }, "control character"},
		{"empty reason", func(in *StartHuntInput) { in.Reason = "" }, "reason"},
		{"empty requester", func(in *StartHuntInput) { in.Requester = "" }, "requester"},
		{"control char in requester", func(in *StartHuntInput) { in.Requester = "bad\x01actor" }, "control character"},
		{"empty approval_id", func(in *StartHuntInput) { in.ApprovalID = "" }, "approval reference"},
		{"invalid approval_id shape", func(in *StartHuntInput) { in.ApprovalID = "not valid!" }, "approval reference"},
		{"invalid label", func(in *StartHuntInput) { in.Label = "windows; SELECT" }, "label"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps, _, _ := testHuntDeps(t)
			handler := newStartHuntHandler(deps)

			in := StartHuntInput{
				CaseID:     "CASE-VAL-1",
				Reason:     "validation test",
				Requester:  "tester",
				ApprovalID: "ref-val-1",
				Artifact:   "Windows.System.Pslist",
				Label:      "windows",
			}
			c.mutate(&in)

			_, _, err := handler(context.Background(), nil, in)
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), c.want)
			}
		})
	}
}

func TestHuntIOCRejectsBadWriteFields(t *testing.T) {
	deps, _, _ := testIOCDeps(t)
	handler := newHuntIOCHandler(deps)

	in := HuntIOCInput{
		CaseID:     "CASE-VAL-2",
		Reason:     "validation test",
		Requester:  "", // required but missing
		ApprovalID: "ref-val-2",
		Kind:       "ip",
		Value:      "192.0.2.1",
		Label:      "linux",
	}
	_, _, err := handler(context.Background(), nil, in)
	if err == nil || !strings.Contains(err.Error(), "requester") {
		t.Fatalf("error = %v, want requester validation failure", err)
	}
}

func TestCancelHuntRejectsMalformedHuntID(t *testing.T) {
	deps, sink, _ := testHuntDeps(t)
	handler := newCancelHuntHandler(deps)

	for _, huntID := range []string{"", "H.1", "H.abc def", "F.1234abcd5678ef90", "H.'; SELECT 1"} {
		_, _, err := handler(context.Background(), nil, CancelHuntInput{
			CaseID:     "CASE-VAL-3",
			Reason:     "validation test",
			Requester:  "tester",
			ApprovalID: "ref-val-3",
			HuntID:     huntID,
		})
		if err == nil || !strings.Contains(err.Error(), "hunt id") {
			t.Errorf("hunt_id %q: error = %v, want invalid hunt id rejection", huntID, err)
		}
	}
	if evt, ok := sink.last(); !ok || evt.Outcome != "blocked" {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}
}

// ---------------------------------------------------------------------------
// S3: nil policy fails closed on hunt/IOC write paths
// ---------------------------------------------------------------------------

func TestNilPolicyDeniesHuntAndIOCWritePaths(t *testing.T) {
	deps, _, _ := testHuntDeps(t)
	deps.Policy = nil
	backendCalled := false
	deps.WriteClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		startHunt: func(_ context.Context, _ velociraptor.HuntRequest) (velociraptor.HuntSummary, error) {
			backendCalled = true
			return velociraptor.HuntSummary{}, nil
		},
		cancelHunt: func(_ context.Context, _ string) error {
			backendCalled = true
			return nil
		},
	}
	deps.VelociraptorWriteMode = VelociraptorModeReal

	startHunt := newStartHuntHandler(deps)
	if _, _, err := startHunt(context.Background(), nil, StartHuntInput{
		CaseID: "CASE-NP-1", Reason: "r", Requester: "t", ApprovalID: "ref-np-1",
		Artifact: "Windows.System.Pslist", Label: "windows",
	}); err == nil {
		t.Error("start hunt with nil policy: expected denial, got nil error")
	}

	cancelHunt := newCancelHuntHandler(deps)
	if _, _, err := cancelHunt(context.Background(), nil, CancelHuntInput{
		CaseID: "CASE-NP-2", Reason: "r", Requester: "t", ApprovalID: "ref-np-2",
		HuntID: "H.1234abcd5678ef90",
	}); err == nil {
		t.Error("cancel hunt with nil policy: expected denial, got nil error")
	}

	huntIOC := newHuntIOCHandler(deps)
	if _, _, err := huntIOC(context.Background(), nil, HuntIOCInput{
		CaseID: "CASE-NP-3", Reason: "r", Requester: "t", ApprovalID: "ref-np-3",
		Kind: "ip", Value: "192.0.2.1", Label: "linux",
	}); err == nil {
		t.Error("hunt ioc with nil policy: expected denial, got nil error")
	}

	if backendCalled {
		t.Error("a Velociraptor write call was made despite nil policy")
	}
}

// ---------------------------------------------------------------------------
// S4: audit sink failure blocks approval-gated writes, preserving the
// approval and never reaching the backend
// ---------------------------------------------------------------------------

func TestAuditWriteFailureBlocksApprovedIOCHunt(t *testing.T) {
	deps, _, store := testIOCDeps(t)

	backendCalled := false
	deps.WriteClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		startHunt: func(_ context.Context, req velociraptor.HuntRequest) (velociraptor.HuntSummary, error) {
			backendCalled = true
			return velociraptor.HuntSummary{HuntID: "H.audit-fail", Artifact: req.Artifact}, nil
		},
	}
	deps.VelociraptorWriteMode = VelociraptorModeReal

	ref := approveRequest(t, store, approval.Request{
		ID:        "ref-audit-fail",
		Operation: approval.OperationHuntIOC,
		CaseID:    "CASE-AF-1",
		Reason:    "audit failure test",
		Requester: "tester",
		Artifact:  "System.IP.Hunt",
		Parameters: map[string]string{
			"IPAddress": "192.0.2.1",
		},
		Label: "linux",
	})

	// Break the audit sink only now, so approval setup above worked with
	// the normal fake sink.
	deps.Audit = failingSink{}

	handler := newHuntIOCHandler(deps)
	_, out, err := handler(context.Background(), nil, HuntIOCInput{
		CaseID:     "CASE-AF-1",
		Reason:     "audit failure test",
		Requester:  "tester",
		ApprovalID: ref,
		Kind:       "ip",
		Value:      "192.0.2.1",
		Label:      "linux",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusError || !strings.Contains(out.Message, "audit log is unavailable") {
		t.Errorf("out = %+v, want error mentioning 'audit log is unavailable'", out)
	}
	if backendCalled {
		t.Error("StartHunt executed despite the audit sink being broken")
	}

	status, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if status.Consumed {
		t.Error("approval was consumed even though the operation was refused for auditability")
	}
}

func TestAuditWriteFailureBlocksApprovedCollection(t *testing.T) {
	deps, _, store := testHuntDeps(t)

	backendCalled := false
	deps.WriteClient = &fakeCollectClient{
		Client: velociraptor.NewClient(),
		collectArtifact: func(_ context.Context, req velociraptor.CollectionRequest) (velociraptor.FlowSummary, error) {
			backendCalled = true
			return velociraptor.FlowSummary{FlowID: "F.audit-fail"}, nil
		},
	}
	deps.VelociraptorWriteMode = VelociraptorModeReal

	ref := approveRequest(t, store, approval.Request{
		ID:        "ref-audit-fail-collect",
		Operation: approval.OperationCollectArtifact,
		CaseID:    "CASE-AF-2",
		Reason:    "audit failure test",
		Requester: "tester",
		ClientID:  "C.1234abcd5678ef90",
		Artifact:  "Windows.System.Pslist",
	})

	deps.Audit = failingSink{}

	handler := newCollectArtifactHandler(deps)
	_, out, err := handler(context.Background(), nil, CollectArtifactInput{
		ClientID:          "C.1234abcd5678ef90",
		Artifact:          "Windows.System.Pslist",
		CaseID:            "CASE-AF-2",
		Reason:            "audit failure test",
		Requester:         "tester",
		ApprovalReference: ref,
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusError || !strings.Contains(out.Message, "audit log is unavailable") {
		t.Errorf("out = %+v, want error mentioning 'audit log is unavailable'", out)
	}
	if backendCalled {
		t.Error("CollectArtifact executed despite the audit sink being broken")
	}

	status, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if status.Consumed {
		t.Error("approval was consumed even though the operation was refused for auditability")
	}
}

// fakeCollectClient overrides just CollectArtifact.
type fakeCollectClient struct {
	velociraptor.Client
	collectArtifact func(ctx context.Context, req velociraptor.CollectionRequest) (velociraptor.FlowSummary, error)
}

func (f *fakeCollectClient) CollectArtifact(ctx context.Context, req velociraptor.CollectionRequest) (velociraptor.FlowSummary, error) {
	return f.collectArtifact(ctx, req)
}
