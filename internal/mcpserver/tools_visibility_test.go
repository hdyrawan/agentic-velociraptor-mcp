package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

func TestHealthCheckHandlerReturnsMockStatus(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newHealthCheckHandler(deps)

	_, out, err := handler(context.Background(), nil, HealthCheckInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if out.Status != "ok" {
		t.Errorf("Status = %q, want %q", out.Status, "ok")
	}
	if out.Mode != "mock" {
		t.Errorf("Mode = %q, want %q", out.Mode, "mock")
	}
	if out.VelociraptorConnected {
		t.Error("VelociraptorConnected = true, want false")
	}
	if out.Message == "" {
		t.Error("Message is empty, want an explanatory message")
	}

	evt, ok := sink.last()
	if !ok {
		t.Fatal("no audit event recorded")
	}
	if evt.Tool != "velo_health_check" {
		t.Errorf("audit Tool = %q, want %q", evt.Tool, "velo_health_check")
	}
	if evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit Outcome = %q, want %q", evt.Outcome, audit.OutcomeSuccess)
	}
}

func TestHealthCheckHandlerRealModeSuccess(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVelociraptorClient{
		Client: velociraptor.NewClient(),
		healthCheck: func(ctx context.Context) (velociraptor.Info, error) {
			return velociraptor.Info{OrgID: "root"}, nil
		},
	}

	handler := newHealthCheckHandler(deps)
	_, out, err := handler(context.Background(), nil, HealthCheckInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if out.Status != "ok" {
		t.Errorf("Status = %q, want %q", out.Status, "ok")
	}
	if out.Mode != "real" {
		t.Errorf("Mode = %q, want %q", out.Mode, "real")
	}
	if !out.VelociraptorConnected {
		t.Error("VelociraptorConnected = false, want true")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event = %+v, ok=%v, want success", evt, ok)
	}
}

func TestHealthCheckHandlerRealModeErrorIsSafeStructuredResult(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVelociraptorClient{
		Client: velociraptor.NewClient(),
		healthCheck: func(ctx context.Context) (velociraptor.Info, error) {
			return velociraptor.Info{}, errors.New("velociraptor: health check: connection refused")
		},
	}

	handler := newHealthCheckHandler(deps)
	res, out, err := handler(context.Background(), nil, HealthCheckInput{})

	// A Velociraptor connectivity failure is data the tool successfully
	// reported, not a tool-level failure: err must be nil and no
	// IsError-shaped result should be forced.
	if err != nil {
		t.Fatalf("handler returned a Go error for a connectivity failure: %v", err)
	}
	if res != nil && res.IsError {
		t.Error("CallToolResult.IsError = true, want a normal structured result")
	}

	if out.Status != "error" {
		t.Errorf("Status = %q, want %q", out.Status, "error")
	}
	if out.Mode != "real" {
		t.Errorf("Mode = %q, want %q", out.Mode, "real")
	}
	if out.VelociraptorConnected {
		t.Error("VelociraptorConnected = true, want false")
	}
	if !strings.Contains(out.Message, "connection refused") {
		t.Errorf("Message = %q, want it to explain the failure", out.Message)
	}
	if strings.Contains(out.Message, "BEGIN") {
		t.Errorf("Message leaks certificate/key content: %q", out.Message)
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError {
		t.Errorf("audit event = %+v, ok=%v, want error outcome", evt, ok)
	}
	if strings.Contains(evt.Reason, "BEGIN") {
		t.Errorf("audit Reason leaks certificate/key content: %q", evt.Reason)
	}
}
