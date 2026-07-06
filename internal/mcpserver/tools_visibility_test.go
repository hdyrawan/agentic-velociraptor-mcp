package mcpserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

// fakeVisibilityClient implements velociraptor.Client by embedding the
// real package's mock placeholder for every method except the four new
// visibility methods, which tests override per-case. Mirrors
// fakeVelociraptorClient's pattern above (mcpserver_test.go), extended
// for the v0.1.0 visibility tools.
type fakeVisibilityClient struct {
	velociraptor.Client
	searchClients      func(ctx context.Context, query string, limit int) ([]velociraptor.ClientSummary, error)
	getClientInfo      func(ctx context.Context, clientID string) (velociraptor.ClientDetail, error)
	listArtifactNames  func(ctx context.Context) ([]velociraptor.ArtifactSummary, error)
	getArtifactDetails func(ctx context.Context, name string) (velociraptor.ArtifactDetail, error)
}

func (f *fakeVisibilityClient) SearchClients(ctx context.Context, query string, limit int) ([]velociraptor.ClientSummary, error) {
	return f.searchClients(ctx, query, limit)
}

func (f *fakeVisibilityClient) GetClientInfo(ctx context.Context, clientID string) (velociraptor.ClientDetail, error) {
	return f.getClientInfo(ctx, clientID)
}

func (f *fakeVisibilityClient) ListArtifactNames(ctx context.Context) ([]velociraptor.ArtifactSummary, error) {
	return f.listArtifactNames(ctx)
}

func (f *fakeVisibilityClient) GetArtifactDetails(ctx context.Context, name string) (velociraptor.ArtifactDetail, error) {
	return f.getArtifactDetails(ctx, name)
}

func TestSearchClientsHandlerMockMode(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newSearchClientsHandler(deps)

	_, out, err := handler(context.Background(), nil, SearchClientsInput{Query: "host"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Mode != "mock" {
		t.Errorf("Mode = %q, want %q", out.Mode, "mock")
	}
	if len(out.Clients) != 0 {
		t.Errorf("Clients = %v, want empty in mock mode", out.Clients)
	}
	if out.Message == "" {
		t.Error("Message is empty, want an explanatory message")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event = %+v, ok=%v, want success", evt, ok)
	}
}

func TestSearchClientsHandlerRealModeSuccess(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVisibilityClient{
		Client: velociraptor.NewClient(),
		searchClients: func(ctx context.Context, query string, limit int) ([]velociraptor.ClientSummary, error) {
			return []velociraptor.ClientSummary{{ClientID: "C.1234abcd5678ef90", Hostname: "WIN-HOST"}}, nil
		},
	}

	handler := newSearchClientsHandler(deps)
	_, out, err := handler(context.Background(), nil, SearchClientsInput{Query: "WIN-HOST"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Mode != "real" {
		t.Errorf("Mode = %q, want %q", out.Mode, "real")
	}
	if len(out.Clients) != 1 || out.Clients[0].ClientID != "C.1234abcd5678ef90" {
		t.Errorf("Clients = %+v, unexpected", out.Clients)
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess || evt.RowCount != 1 {
		t.Errorf("audit event = %+v, ok=%v, want success with row_count=1", evt, ok)
	}
}

func TestSearchClientsHandlerRealModeErrorIsSafeStructuredResult(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVisibilityClient{
		Client: velociraptor.NewClient(),
		searchClients: func(ctx context.Context, query string, limit int) ([]velociraptor.ClientSummary, error) {
			return nil, errors.New("velociraptor: search clients: connection refused")
		},
	}

	handler := newSearchClientsHandler(deps)
	res, out, err := handler(context.Background(), nil, SearchClientsInput{})
	if err != nil {
		t.Fatalf("handler returned a Go error for a connectivity failure: %v", err)
	}
	if res != nil && res.IsError {
		t.Error("CallToolResult.IsError = true, want a normal structured result")
	}
	if out.Mode != "real" {
		t.Errorf("Mode = %q, want %q", out.Mode, "real")
	}
	if !strings.Contains(out.Message, "connection refused") {
		t.Errorf("Message = %q, want it to explain the failure", out.Message)
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError {
		t.Errorf("audit event = %+v, ok=%v, want error outcome", evt, ok)
	}
}

func TestSearchClientsHandlerRejectsInvalidQuery(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newSearchClientsHandler(deps)

	_, _, err := handler(context.Background(), nil, SearchClientsInput{Query: "bad\x00query"})
	if err == nil {
		t.Fatal("handler: expected error for control-character query, got nil")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event = %+v, ok=%v, want blocked outcome", evt, ok)
	}
}

func TestGetClientInfoHandlerRejectsInvalidClientID(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newGetClientInfoHandler(deps)

	_, _, err := handler(context.Background(), nil, GetClientInfoInput{ClientID: "not-a-client-id"})
	if err == nil {
		t.Fatal("handler: expected error for malformed client id, got nil")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event = %+v, ok=%v, want blocked outcome", evt, ok)
	}
}

func TestGetClientInfoHandlerMockMode(t *testing.T) {
	deps, _ := testDeps(t)
	handler := newGetClientInfoHandler(deps)

	_, out, err := handler(context.Background(), nil, GetClientInfoInput{ClientID: "C.1234abcd5678ef90"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Mode != "mock" {
		t.Errorf("Mode = %q, want %q", out.Mode, "mock")
	}
	if out.Client != nil {
		t.Errorf("Client = %+v, want nil in mock mode", out.Client)
	}
}

func TestGetClientInfoHandlerRealModeSuccess(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVisibilityClient{
		Client: velociraptor.NewClient(),
		getClientInfo: func(ctx context.Context, clientID string) (velociraptor.ClientDetail, error) {
			return velociraptor.ClientDetail{
				ClientSummary: velociraptor.ClientSummary{ClientID: clientID, Hostname: "srv1"},
				Labels:        []string{"triage"},
			}, nil
		},
	}

	handler := newGetClientInfoHandler(deps)
	_, out, err := handler(context.Background(), nil, GetClientInfoInput{ClientID: "C.1234abcd5678ef90"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Client == nil || out.Client.Hostname != "srv1" || len(out.Client.Labels) != 1 {
		t.Errorf("Client = %+v, unexpected", out.Client)
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess || evt.ClientID != "C.1234abcd5678ef90" {
		t.Errorf("audit event = %+v, ok=%v, want success with client_id set", evt, ok)
	}
}

// TestGetClientInfoHandlerRealModeErrorDoesNotLeakSecrets checks that the
// handler layer forwards a Velociraptor-layer error's message verbatim
// into Message/Reason without reformatting it in a way that could
// reintroduce secret content. Actual PEM/key redaction is
// internal/velociraptor.sanitizeTLSError's job and is exercised directly
// in grpcclient_test.go (e.g. TestGRPCClientGetClientInfoErrorDoesNotLeakSecrets);
// this fake simulates the already-sanitized error grpcClient would have
// produced, since velociraptor.Client is documented to never return raw
// key material to its callers.
func TestGetClientInfoHandlerRealModeErrorDoesNotLeakSecrets(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVisibilityClient{
		Client: velociraptor.NewClient(),
		getClientInfo: func(ctx context.Context, clientID string) (velociraptor.ClientDetail, error) {
			return velociraptor.ClientDetail{}, errors.New("velociraptor: get client info: tls: bad certificate: [REDACTED]")
		},
	}

	handler := newGetClientInfoHandler(deps)
	_, out, err := handler(context.Background(), nil, GetClientInfoInput{ClientID: "C.1234abcd5678ef90"})
	if err != nil {
		t.Fatalf("handler returned a Go error for a connectivity failure: %v", err)
	}
	if strings.Contains(out.Message, "BEGIN") || strings.Contains(out.Message, "secret") {
		t.Errorf("Message leaks certificate content: %q", out.Message)
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError {
		t.Errorf("audit event = %+v, ok=%v, want error outcome", evt, ok)
	}
	if strings.Contains(evt.Reason, "BEGIN") || strings.Contains(evt.Reason, "secret") {
		t.Errorf("audit Reason leaks certificate content: %q", evt.Reason)
	}
}

func TestListArtifactNamesHandlerMockMode(t *testing.T) {
	deps, _ := testDeps(t)
	handler := newListArtifactNamesHandler(deps)

	_, out, err := handler(context.Background(), nil, ListArtifactNamesInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Mode != "mock" {
		t.Errorf("Mode = %q, want %q", out.Mode, "mock")
	}
	if len(out.Artifacts) != 0 {
		t.Errorf("Artifacts = %v, want empty in mock mode", out.Artifacts)
	}
}

func TestListArtifactNamesHandlerFiltersToAllowlistByDefault(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVisibilityClient{
		Client: velociraptor.NewClient(),
		listArtifactNames: func(ctx context.Context) ([]velociraptor.ArtifactSummary, error) {
			return []velociraptor.ArtifactSummary{
				{Name: "Generic.Client.Info", Description: "allowlisted"},
				{Name: "Windows.EventLogs.RDPAuth", Description: "not allowlisted by config.Default()"},
			}, nil
		},
	}

	handler := newListArtifactNamesHandler(deps)
	_, out, err := handler(context.Background(), nil, ListArtifactNamesInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(out.Artifacts) != 1 || out.Artifacts[0].Name != "Generic.Client.Info" {
		t.Errorf("Artifacts = %+v, want only the allowlisted entry (config.Default() allows Generic.Client.Info, Windows.System.Pslist, Windows.Network.Netstat)", out.Artifacts)
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess || evt.RowCount != 1 {
		t.Errorf("audit event = %+v, ok=%v, want success with row_count=1", evt, ok)
	}
}

func TestGetArtifactDetailsHandlerRejectsInvalidName(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newGetArtifactDetailsHandler(deps)

	_, _, err := handler(context.Background(), nil, GetArtifactDetailsInput{Name: "Not Valid; DROP"})
	if err == nil {
		t.Fatal("handler: expected error for malformed artifact name, got nil")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event = %+v, ok=%v, want blocked outcome", evt, ok)
	}
}

func TestGetArtifactDetailsHandlerBlocksNonAllowlistedArtifact(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newGetArtifactDetailsHandler(deps)

	// Windows.EventLogs.RDPAuth is syntactically valid but not present in
	// config.Default()'s allowed_artifacts list.
	_, _, err := handler(context.Background(), nil, GetArtifactDetailsInput{Name: "Windows.EventLogs.RDPAuth"})
	if err == nil {
		t.Fatal("handler: expected error for non-allowlisted artifact, got nil")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event = %+v, ok=%v, want blocked outcome", evt, ok)
	}
}

func TestGetArtifactDetailsHandlerRealModeSuccess(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVisibilityClient{
		Client: velociraptor.NewClient(),
		getArtifactDetails: func(ctx context.Context, name string) (velociraptor.ArtifactDetail, error) {
			return velociraptor.ArtifactDetail{
				ArtifactSummary: velociraptor.ArtifactSummary{Name: name, Description: "Basic client info"},
				Parameters:      []velociraptor.ArtifactParameter{{Name: "Foo", Type: "string"}},
			}, nil
		},
	}

	handler := newGetArtifactDetailsHandler(deps)
	_, out, err := handler(context.Background(), nil, GetArtifactDetailsInput{Name: "Generic.Client.Info"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Artifact == nil || out.Artifact.Name != "Generic.Client.Info" || len(out.Artifact.Parameters) != 1 {
		t.Errorf("Artifact = %+v, unexpected", out.Artifact)
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess || evt.Artifact != "Generic.Client.Info" {
		t.Errorf("audit event = %+v, ok=%v, want success with artifact set", evt, ok)
	}
}

// TestGetArtifactDetailsHandlerRealModeErrorDoesNotLeakSecrets checks
// that the handler layer forwards a Velociraptor-layer error's message
// verbatim without reintroducing secret content; see the doc comment on
// TestGetClientInfoHandlerRealModeErrorDoesNotLeakSecrets for why the
// fake error here is already sanitized.
func TestGetArtifactDetailsHandlerRealModeErrorDoesNotLeakSecrets(t *testing.T) {
	deps, sink := testDeps(t)
	deps.VelociraptorReadMode = VelociraptorModeReal
	deps.ReadClient = &fakeVisibilityClient{
		Client: velociraptor.NewClient(),
		getArtifactDetails: func(ctx context.Context, name string) (velociraptor.ArtifactDetail, error) {
			return velociraptor.ArtifactDetail{}, errors.New("velociraptor: get artifact details: tls: bad certificate: [REDACTED]")
		},
	}

	handler := newGetArtifactDetailsHandler(deps)
	_, out, err := handler(context.Background(), nil, GetArtifactDetailsInput{Name: "Generic.Client.Info"})
	if err != nil {
		t.Fatalf("handler returned a Go error for a connectivity failure: %v", err)
	}
	if strings.Contains(out.Message, "BEGIN") || strings.Contains(out.Message, "secret") {
		t.Errorf("Message leaks key material: %q", out.Message)
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError {
		t.Errorf("audit event = %+v, ok=%v, want error outcome", evt, ok)
	}
	if strings.Contains(evt.Reason, "BEGIN") || strings.Contains(evt.Reason, "secret") {
		t.Errorf("audit Reason leaks key material: %q", evt.Reason)
	}
}

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
