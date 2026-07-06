package mcpserver

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/config"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/dfir"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/policy"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

// fakeAuditSink records every audit.Event written to it, for assertions
// in tests. Safe for concurrent use, though tests here are sequential.
type fakeAuditSink struct {
	mu     sync.Mutex
	events []audit.Event
}

func (f *fakeAuditSink) Write(evt audit.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, evt)
	return nil
}

func (f *fakeAuditSink) Close() error { return nil }

func (f *fakeAuditSink) last() (audit.Event, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.events) == 0 {
		return audit.Event{}, false
	}
	return f.events[len(f.events)-1], true
}

// testDeps builds a Deps backed by the real shipped profiles/ directory
// and a conservative default policy, with a fakeAuditSink so tests can
// assert on audit outcomes.
func testDeps(t interface{ Fatalf(string, ...any) }) (Deps, *fakeAuditSink) {
	reg, err := dfir.LoadDir("../../profiles")
	if err != nil {
		t.Fatalf("dfir.LoadDir: %v", err)
	}

	cfg := config.Default()
	sink := &fakeAuditSink{}

	return Deps{
		Config:   cfg,
		Policy:   policy.NewEngine(cfg.Policy),
		Audit:    sink,
		Profiles: reg,
	}, sink
}

// fakeVelociraptorClient implements velociraptor.Client by embedding
// the real package's mock placeholder (velociraptor.NewClient()) for
// every method, and lets tests override HealthCheck. This is the same
// "testable with both mock and real-client interfaces" pattern
// production code uses (grpcClient embeds placeholderClient and
// overrides HealthCheck), applied here so mcpserver tests can drive
// velo_health_check's real-mode branch without any TLS/network setup.
type fakeVelociraptorClient struct {
	velociraptor.Client
	healthCheck        func(ctx context.Context) (velociraptor.Info, error)
	collectArtifact    func(ctx context.Context, req velociraptor.CollectionRequest) (velociraptor.FlowSummary, error)
	cancelFlow         func(ctx context.Context, clientID, flowID string) error
	listFlowUploads    func(ctx context.Context, clientID, flowID string) ([]velociraptor.UploadSummary, error)
	getUploadMetadata  func(ctx context.Context, clientID, flowID, uploadName string) (velociraptor.UploadSummary, error)
	downloadFlowUpload func(ctx context.Context, clientID, flowID, uploadName string, maxBytes int64) ([]byte, error)
}

func (f *fakeVelociraptorClient) HealthCheck(ctx context.Context) (velociraptor.Info, error) {
	return f.healthCheck(ctx)
}

func (f *fakeVelociraptorClient) CollectArtifact(ctx context.Context, req velociraptor.CollectionRequest) (velociraptor.FlowSummary, error) {
	return f.collectArtifact(ctx, req)
}

func (f *fakeVelociraptorClient) CancelFlow(ctx context.Context, clientID, flowID string) error {
	return f.cancelFlow(ctx, clientID, flowID)
}

func (f *fakeVelociraptorClient) ListFlowUploads(ctx context.Context, clientID, flowID string) ([]velociraptor.UploadSummary, error) {
	return f.listFlowUploads(ctx, clientID, flowID)
}

func (f *fakeVelociraptorClient) GetFlowUploadMetadata(ctx context.Context, clientID, flowID, uploadName string) (velociraptor.UploadSummary, error) {
	return f.getUploadMetadata(ctx, clientID, flowID, uploadName)
}

func (f *fakeVelociraptorClient) DownloadFlowUpload(ctx context.Context, clientID, flowID, uploadName string, maxBytes int64) ([]byte, error) {
	return f.downloadFlowUpload(ctx, clientID, flowID, uploadName, maxBytes)
}

// testWritePilotDeps builds a Deps identical to testDeps but with the
// write pilot enabled: Policy.Mode is "controlled" and Approvals is a
// real approval.FileStore backed by a temp file, so tests can create and
// decide approval.Request records exactly as the out-of-band `approve`
// CLI would, then exercise tool handlers against them.
func testWritePilotDeps(t interface {
	Fatalf(string, ...any)
	TempDir() string
}) (Deps, *fakeAuditSink) {
	deps, sink := testDeps(t)

	cfg := config.Default()
	cfg.Policy.Mode = config.PolicyModeControlled
	cfg.Approval.StorePath = filepath.Join(t.TempDir(), "approvals.json")
	cfg.Approval.TTLSeconds = 900
	deps.Config = cfg
	deps.Policy = policy.NewEngine(cfg.Policy)

	store, err := approval.NewFileStore(cfg.Approval.StorePath, time.Duration(cfg.Approval.TTLSeconds)*time.Second)
	if err != nil {
		t.Fatalf("approval.NewFileStore: %v", err)
	}
	deps.Approvals = store

	return deps, sink
}

// approveRequest creates and approves req against store, as the
// out-of-band `approve` CLI would, and returns the approval reference.
// Test-only: no MCP tool handler can do this.
func approveRequest(t interface{ Fatalf(string, ...any) }, store approval.Store, req approval.Request) string {
	created, err := store.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("approval.Store.Create: %v", err)
	}
	if err := store.Decide(context.Background(), approval.Decision{RequestID: created.ID, Approved: true, ApprovedBy: "test-approver"}); err != nil {
		t.Fatalf("approval.Store.Decide: %v", err)
	}
	return created.ID
}
