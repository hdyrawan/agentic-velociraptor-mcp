package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/config"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/policy"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

// iocAllowedArtifacts lists every illustrative artifact vql.Bind resolves
// an IOC kind to, so tests can allowlist them all without needing to
// import internal/vql (this package doesn't otherwise depend on it).
var iocAllowedArtifacts = []string{
	"System.Hash.Hunt",
	"System.IP.Hunt",
	"System.Domain.Hunt",
	"System.Process.Hunt",
	"System.Path.Hunt",
}

// testIOCDeps builds a Deps with the write pilot enabled by default
// (PolicyModeControlled + a real approval.FileStore) and every
// illustrative IOC artifact allowlisted, mirroring testHuntDeps so
// fingerprint matching is exercised exactly as production code enforces
// it. Tests that need read-only mode override deps.Policy afterward.
func testIOCDeps(t *testing.T) (Deps, *fakeAuditSink, approval.Store) {
	cfg := config.Default()
	cfg.Policy.Mode = config.PolicyModeControlled
	cfg.Policy.AllowedArtifacts = append([]string{}, iocAllowedArtifacts...)
	cfg.Policy.MaxHuntClients = 50
	cfg.Approval.StorePath = filepath.Join(t.TempDir(), "approvals.json")
	cfg.Approval.TTLSeconds = 900

	store, err := approval.NewFileStore(cfg.Approval.StorePath, time.Duration(cfg.Approval.TTLSeconds)*time.Second)
	if err != nil {
		t.Fatalf("approval.NewFileStore: %v", err)
	}

	sink := &fakeAuditSink{}
	return Deps{
		Config:    cfg,
		Policy:    policy.NewEngine(cfg.Policy),
		Audit:     sink,
		Approvals: store,
	}, sink, store
}

// ---------------------------------------------------------------------------
// IOC kind/value validation
// ---------------------------------------------------------------------------

func TestHuntIOCValidatesEachKind(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		value   string
		wantErr bool
	}{
		{"valid hash", "hash", "d41d8cd98f00b204e9800998ecf8427e", false},
		{"invalid hash", "hash", "not-a-hash", true},
		{"valid ip", "ip", "192.0.2.1", false},
		{"invalid ip", "ip", "not-an-ip", true},
		{"valid domain", "domain", "evil.example.com", false},
		{"invalid domain", "domain", "not a domain", true},
		{"valid process", "process", "svchost.exe", false},
		{"invalid process", "process", "/usr/bin/bash", true},
		{"valid path", "path", "/usr/bin/bash", false},
		{"invalid path", "path", "../etc/passwd", true},
		{"unknown kind", "bogus", "whatever", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps, sink, _ := testIOCDeps(t)
			handler := newHuntIOCHandler(deps)

			_, out, err := handler(context.Background(), nil, HuntIOCInput{
				CaseID:     "CASE-IOC-1",
				Reason:     "test ioc validation",
				Requester:  "tester",
				ApprovalID: "approval-nonexistent",
				Kind:       c.kind,
				Value:      c.value,
				Label:      "linux",
			})

			if c.wantErr {
				if err == nil {
					t.Fatalf("expected validation error for kind=%q value=%q, got nil", c.kind, c.value)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected Go error for kind=%q value=%q: %v", c.kind, c.value, err)
				}
				// Valid input proceeds past validation to the (missing)
				// approval check, so the handler must report not_found,
				// not a validation-shaped error.
				if out.Status != response.StatusNotFound {
					t.Errorf("out = %+v, want not_found (validation passed, approval missing)", out)
				}
			}
			evt, ok := sink.last()
			if !ok || evt.Outcome != audit.OutcomeBlocked {
				t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Policy gates
// ---------------------------------------------------------------------------

func TestHuntIOCBlockedInReadOnlyMode(t *testing.T) {
	deps, sink, _ := testIOCDeps(t)
	deps.Policy = policy.NewEngine(config.PolicyConfig{
		Mode:             config.PolicyModeReadOnly,
		AllowedArtifacts: append([]string{}, iocAllowedArtifacts...),
	})

	handler := newHuntIOCHandler(deps)
	_, _, err := handler(context.Background(), nil, HuntIOCInput{
		CaseID:     "CASE-IOC-2",
		Reason:     "test read-only",
		Requester:  "tester",
		ApprovalID: "approval-ok",
		Kind:       "hash",
		Value:      "d41d8cd98f00b204e9800998ecf8427e",
	})
	if err == nil {
		t.Fatal("expected error in read-only mode, got nil")
	}
	if !strings.Contains(err.Error(), `must be "controlled"`) {
		t.Errorf("error = %q, want policy.mode must be \"controlled\"", err.Error())
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}
}

func TestHuntIOCBlockedWithoutApproval(t *testing.T) {
	deps, sink, _ := testIOCDeps(t)
	handler := newHuntIOCHandler(deps)

	_, out, err := handler(context.Background(), nil, HuntIOCInput{
		CaseID:     "CASE-IOC-3",
		Reason:     "test no approval",
		Requester:  "tester",
		ApprovalID: "approval-nonexistent",
		Kind:       "ip",
		Value:      "192.0.2.1",
		Label:      "linux",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusNotFound || !strings.Contains(out.Message, "was not found") {
		t.Errorf("out = %+v, want not_found mentioning 'was not found'", out)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}
}

func TestHuntIOCTargetAllBlockedByDefault(t *testing.T) {
	deps, sink, store := testIOCDeps(t)

	ref := approveRequest(t, store, approval.Request{
		ID:        "ref-ioc-all",
		Operation: approval.OperationHuntIOC,
		CaseID:    "CASE-IOC-4",
		Reason:    "test target_all",
		Requester: "tester",
		Artifact:  "System.IP.Hunt",
		Parameters: map[string]string{
			"IPAddress": "192.0.2.1",
		},
		TargetAll: true,
	})

	handler := newHuntIOCHandler(deps)
	_, _, err := handler(context.Background(), nil, HuntIOCInput{
		CaseID:     "CASE-IOC-4",
		Reason:     "test target_all",
		Requester:  "tester",
		ApprovalID: ref,
		Kind:       "ip",
		Value:      "192.0.2.1",
		All:        true,
	})
	if err == nil {
		t.Fatal("expected error for target_all, got nil")
	}
	if !strings.Contains(err.Error(), "target_all is disabled by policy") {
		t.Errorf("error = %q, want 'target_all is disabled by policy'", err.Error())
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}

	// target_all was rejected before the approval check was ever reached,
	// so the approval must remain unconsumed.
	status, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if status.Consumed {
		t.Error("approval was consumed despite being blocked by target_all policy")
	}
}

func TestHuntIOCEnforcesMaxHuntClients(t *testing.T) {
	deps, _, store := testIOCDeps(t)
	deps.Policy = policy.NewEngine(config.PolicyConfig{
		Mode:             config.PolicyModeControlled,
		MaxHuntClients:   5,
		AllowedArtifacts: append([]string{}, iocAllowedArtifacts...),
	})

	var capturedMaxClients int
	deps.WriteClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		startHunt: func(_ context.Context, req velociraptor.HuntRequest) (velociraptor.HuntSummary, error) {
			capturedMaxClients = req.MaxClients
			return velociraptor.HuntSummary{HuntID: "H.ioc", Artifact: req.Artifact, State: velociraptor.HuntStateRunning}, nil
		},
	}
	deps.VelociraptorWriteMode = VelociraptorModeReal

	ref := approveRequest(t, store, approval.Request{
		ID:        "ref-ioc-maxclients",
		Operation: approval.OperationHuntIOC,
		CaseID:    "CASE-IOC-5",
		Reason:    "test max clients",
		Requester: "tester",
		Artifact:  "System.Hash.Hunt",
		Parameters: map[string]string{
			"HashValue": "d41d8cd98f00b204e9800998ecf8427e",
		},
		Label: "linux",
	})

	handler := newHuntIOCHandler(deps)
	_, out, err := handler(context.Background(), nil, HuntIOCInput{
		CaseID:     "CASE-IOC-5",
		Reason:     "test max clients",
		Requester:  "tester",
		ApprovalID: ref,
		Kind:       "hash",
		Value:      "d41d8cd98f00b204e9800998ecf8427e",
		Label:      "linux",
		MaxClients: 100, // requests more than the policy ceiling of 5
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusSuccess)
	}
	if capturedMaxClients != 5 {
		t.Errorf("MaxClients sent to StartHunt = %d, want 5 (policy ceiling, caller cannot raise it)", capturedMaxClients)
	}
}

// ---------------------------------------------------------------------------
// Confused-deputy / fingerprint regressions
// ---------------------------------------------------------------------------

func TestHuntIOCRejectsMismatchedApproval(t *testing.T) {
	deps, sink, store := testIOCDeps(t)
	deps.WriteClient = velociraptor.NewClient()
	deps.VelociraptorWriteMode = VelociraptorModeReal

	// Approved for a hash indicator...
	ref := approveRequest(t, store, approval.Request{
		ID:        "ref-ioc-mismatch",
		Operation: approval.OperationHuntIOC,
		CaseID:    "CASE-IOC-6",
		Reason:    "test ioc mismatch",
		Requester: "tester",
		Artifact:  "System.Hash.Hunt",
		Parameters: map[string]string{
			"HashValue": "d41d8cd98f00b204e9800998ecf8427e",
		},
		Label: "linux",
	})

	handler := newHuntIOCHandler(deps)
	// ...but the call actually hunts a different hash value.
	_, out, err := handler(context.Background(), nil, HuntIOCInput{
		CaseID:     "CASE-IOC-6",
		Reason:     "test ioc mismatch",
		Requester:  "tester",
		ApprovalID: ref,
		Kind:       "hash",
		Value:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Label:      "linux",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusError || !strings.Contains(out.Message, "does not match") {
		t.Errorf("out = %+v, want error mentioning 'does not match'", out)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}

	status, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if status.Consumed {
		t.Error("mismatched approval must not be consumed")
	}
}

// TestApprovalForIOCHuntCannotAuthorizeRegularHuntStart proves an
// approval created for velo_hunt_ioc_with_approval (OperationHuntIOC)
// cannot be replayed against velo_start_hunt_with_approval
// (OperationStartHunt), even when case_id and artifact happen to line
// up: RequestFingerprint includes Operation, so the two tools' requests
// can never collide.
func TestApprovalForIOCHuntCannotAuthorizeRegularHuntStart(t *testing.T) {
	deps, _, store := testIOCDeps(t)
	deps.Policy = policy.NewEngine(config.PolicyConfig{
		Mode:             config.PolicyModeControlled,
		MaxHuntClients:   50,
		AllowedArtifacts: append(append([]string{}, iocAllowedArtifacts...), "System.Hash.Hunt"),
	})
	deps.WriteClient = velociraptor.NewClient()
	deps.VelociraptorWriteMode = VelociraptorModeReal

	ref := approveRequest(t, store, approval.Request{
		ID:        "ref-cross-tool",
		Operation: approval.OperationHuntIOC,
		CaseID:    "CASE-IOC-7",
		Reason:    "ioc hunt for a hash",
		Requester: "tester",
		Artifact:  "System.Hash.Hunt",
		Parameters: map[string]string{
			"HashValue": "d41d8cd98f00b204e9800998ecf8427e",
		},
		Label: "linux",
	})

	startHuntHandler := newStartHuntHandler(deps)
	_, out, err := startHuntHandler(context.Background(), nil, StartHuntInput{
		CaseID:     "CASE-IOC-7",
		Reason:     "ioc hunt for a hash",
		Requester:  "tester",
		ApprovalID: ref,
		Artifact:   "System.Hash.Hunt",
		Label:      "linux",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusError || !strings.Contains(out.Message, "does not match") {
		t.Errorf("out = %+v, want error mentioning 'does not match' (IOC approval must not authorize a plain hunt start)", out)
	}

	status, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if status.Consumed {
		t.Error("IOC approval must not be consumable by a different tool/operation")
	}
}

// ---------------------------------------------------------------------------
// Approved path / scaffolded real-mode honesty
// ---------------------------------------------------------------------------

func TestHuntIOCApprovedFakePath(t *testing.T) {
	deps, sink, store := testIOCDeps(t)
	deps.WriteClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		startHunt: func(_ context.Context, req velociraptor.HuntRequest) (velociraptor.HuntSummary, error) {
			return velociraptor.HuntSummary{
				HuntID: "H.ioc-hash", Artifact: req.Artifact, State: velociraptor.HuntStateRunning,
			}, nil
		},
	}
	deps.VelociraptorWriteMode = VelociraptorModeReal

	ref := approveRequest(t, store, approval.Request{
		ID:        "ref-ioc-ok",
		Operation: approval.OperationHuntIOC,
		CaseID:    "CASE-IOC-8",
		Reason:    "approved ioc hunt test",
		Requester: "tester",
		Artifact:  "System.Domain.Hunt",
		Parameters: map[string]string{
			"Domain": "evil.example.com",
		},
		Label: "linux",
	})

	handler := newHuntIOCHandler(deps)
	_, out, err := handler(context.Background(), nil, HuntIOCInput{
		CaseID:     "CASE-IOC-8",
		Reason:     "approved ioc hunt test",
		Requester:  "tester",
		ApprovalID: ref,
		Kind:       "domain",
		Value:      "evil.example.com",
		Label:      "linux",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Errorf("Status = %q, want %q", out.Status, response.StatusSuccess)
	}
	if out.HuntID != "H.ioc-hash" {
		t.Errorf("HuntID = %q, want H.ioc-hash", out.HuntID)
	}
	if out.Artifact != "System.Domain.Hunt" {
		t.Errorf("Artifact = %q, want System.Domain.Hunt", out.Artifact)
	}

	status, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if !status.Consumed {
		t.Error("Consume was not called on the approval store")
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event outcome = %q, want success", evt.Outcome)
	}
}

// TestHuntIOCRealModeExplicitClientIDsPreservesApproval proves that in
// real backend mode, an explicit client_ids hunt scope is refused before
// gateAuditForWrite/consumeApproval, not after — so the one-shot approval
// survives a request Velociraptor's typed hunt RPCs can never actually
// enact (see velociraptor.ErrHuntScopeClientIDsUnsupported).
// WriteClient.StartHunt must never be called for this case.
func TestHuntIOCRealModeExplicitClientIDsPreservesApproval(t *testing.T) {
	deps, sink, store := testIOCDeps(t)
	deps.WriteClient = &fakeHuntClient{
		Client: velociraptor.NewClient(),
		startHunt: func(_ context.Context, req velociraptor.HuntRequest) (velociraptor.HuntSummary, error) {
			t.Fatal("StartHunt must not be called for an explicit client_ids scope in real mode")
			return velociraptor.HuntSummary{}, nil
		},
	}
	deps.VelociraptorWriteMode = VelociraptorModeReal

	ref := approveRequest(t, store, approval.Request{
		ID:        "ref-ioc-clientids",
		Operation: approval.OperationHuntIOC,
		CaseID:    "CASE-IOC-CLIENTIDS",
		Reason:    "explicit client ids regression",
		Requester: "tester",
		Artifact:  "System.Domain.Hunt",
		Parameters: map[string]string{
			"Domain": "evil.example.com",
		},
		ClientIDs: []string{"C.1234abcd5678ef90"},
	})

	handler := newHuntIOCHandler(deps)
	_, out, err := handler(context.Background(), nil, HuntIOCInput{
		CaseID:     "CASE-IOC-CLIENTIDS",
		Reason:     "explicit client ids regression",
		Requester:  "tester",
		ApprovalID: ref,
		Kind:       "domain",
		Value:      "evil.example.com",
		ClientIDs:  []string{"C.1234abcd5678ef90"},
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusError || !strings.Contains(out.Message, "client_ids") {
		t.Fatalf("out = %+v, want an error mentioning client_ids", out)
	}

	status, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if status.Consumed {
		t.Error("approval must remain unconsumed when explicit client_ids scope is unsupported")
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event outcome = %q, want blocked", evt.Outcome)
	}
}

// TestHuntIOCScaffoldedRealModeReturnsHonestError confirms that when
// WriteClient is a real (non-mock) client without hunt RPCs implemented
// (placeholderClient, embedded by grpcClient), the tool reports the
// underlying scaffolded backend gap honestly as an error-status result
// rather than fabricating success or consuming approval before the
// backend gate passes.
func TestHuntIOCScaffoldedRealModeReturnsHonestError(t *testing.T) {
	deps, sink, store := testIOCDeps(t)
	deps.WriteClient = velociraptor.NewClient() // placeholderClient: StartHunt -> ErrNotImplemented
	deps.VelociraptorWriteMode = VelociraptorModeReal

	ref := approveRequest(t, store, approval.Request{
		ID:        "ref-ioc-scaffold",
		Operation: approval.OperationHuntIOC,
		CaseID:    "CASE-IOC-9",
		Reason:    "scaffolded real mode",
		Requester: "tester",
		Artifact:  "System.Path.Hunt",
		Parameters: map[string]string{
			"Path": "/usr/bin/bash",
		},
		Label: "linux",
	})

	handler := newHuntIOCHandler(deps)
	_, out, err := handler(context.Background(), nil, HuntIOCInput{
		CaseID:     "CASE-IOC-9",
		Reason:     "scaffolded real mode",
		Requester:  "tester",
		ApprovalID: ref,
		Kind:       "path",
		Value:      "/usr/bin/bash",
		Label:      "linux",
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out.Status != response.StatusError {
		t.Errorf("Status = %q, want %q (honest scaffolded failure, not fabricated success)", out.Status, response.StatusError)
	}
	if !strings.Contains(out.Message, "not implemented") {
		t.Errorf("Message = %q, want it to mention ErrNotImplemented", out.Message)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError {
		t.Errorf("audit event outcome = %q, want error", evt.Outcome)
	}

	// The approval was not consumed: v0.8.0 runs backend-capability gates
	// before burning one-shot approvals, so a scaffolded path preserves the
	// approval for a later build that can actually execute it.
	status, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if status.Consumed {
		t.Error("approval was consumed even though backend support failed before execution")
	}
}

// ---------------------------------------------------------------------------
// No raw VQL exposed
// ---------------------------------------------------------------------------

func TestHuntIOCToolRegisteredWithoutRawVQL(t *testing.T) {
	deps, _, _ := testIOCDeps(t)
	srv := New("test", "0.0.0", deps)

	session := connectTestClient(t, srv)
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	var found bool
	for _, tool := range res.Tools {
		name := strings.ToLower(tool.Name)
		if strings.Contains(name, "vql") {
			t.Errorf("tool %q contains a VQL pattern in its name", tool.Name)
		}
		if tool.Name == "velo_hunt_ioc_with_approval" {
			found = true
			if tool.Annotations == nil || tool.Annotations.ReadOnlyHint {
				t.Errorf("velo_hunt_ioc_with_approval annotations = %+v, want write-capable (ReadOnlyHint=false)", tool.Annotations)
			}
		}
	}
	if !found {
		t.Error("velo_hunt_ioc_with_approval is not registered")
	}
}
