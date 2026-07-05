package mcpserver

import (
	"context"
	"sync"

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
	healthCheck func(ctx context.Context) (velociraptor.Info, error)
}

func (f *fakeVelociraptorClient) HealthCheck(ctx context.Context) (velociraptor.Info, error) {
	return f.healthCheck(ctx)
}
