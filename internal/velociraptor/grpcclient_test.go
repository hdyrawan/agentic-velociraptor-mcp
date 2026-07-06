package velociraptor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor/veloapi"
)

// fakeHealthChecker implements healthChecker without any real network or
// TLS setup, so grpcClient.HealthCheck's timeout/error-handling logic
// can be tested directly.
type fakeHealthChecker struct {
	resp    *veloapi.HealthCheckResponse
	err     error
	delay   time.Duration
	sawCtx  context.Context
	sawCall bool
}

func (f *fakeHealthChecker) Check(ctx context.Context, in *veloapi.HealthCheckRequest, opts ...grpc.CallOption) (*veloapi.HealthCheckResponse, error) {
	f.sawCall = true
	f.sawCtx = ctx
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.resp, f.err
}

func TestGRPCClientHealthCheckSuccess(t *testing.T) {
	fake := &fakeHealthChecker{
		resp: &veloapi.HealthCheckResponse{Status: veloapi.HealthCheckResponse_SERVING},
	}
	c := &grpcClient{health: fake, timeout: time.Second, orgID: "root"}

	info, err := c.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck: %v", err)
	}
	if info.OrgID != "root" {
		t.Errorf("Info.OrgID = %q, want %q", info.OrgID, "root")
	}
	if !fake.sawCall {
		t.Error("Check was never called")
	}
}

func TestGRPCClientHealthCheckNotServing(t *testing.T) {
	fake := &fakeHealthChecker{
		resp: &veloapi.HealthCheckResponse{Status: veloapi.HealthCheckResponse_NOT_SERVING},
	}
	c := &grpcClient{health: fake, timeout: time.Second}

	_, err := c.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck: expected error for NOT_SERVING status, got nil")
	}
}

func TestGRPCClientHealthCheckTransportError(t *testing.T) {
	fake := &fakeHealthChecker{err: errors.New("connection refused")}
	c := &grpcClient{health: fake, timeout: time.Second}

	_, err := c.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %v, want it to mention the underlying transport error", err)
	}
}

func TestGRPCClientHealthCheckTimeout(t *testing.T) {
	fake := &fakeHealthChecker{
		resp:  &veloapi.HealthCheckResponse{Status: veloapi.HealthCheckResponse_SERVING},
		delay: 50 * time.Millisecond,
	}
	c := &grpcClient{health: fake, timeout: 5 * time.Millisecond}

	start := time.Now()
	_, err := c.HealthCheck(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("HealthCheck: expected timeout error, got nil")
	}
	if elapsed > 40*time.Millisecond {
		t.Errorf("HealthCheck took %v, expected it to respect the 5ms timeout rather than the fake's 50ms delay", elapsed)
	}
}

func TestGRPCClientHealthCheckErrorDoesNotLeakSecrets(t *testing.T) {
	fake := &fakeHealthChecker{
		err: errors.New("tls: bad certificate: -----BEGIN RSA PRIVATE KEY-----\nMIIEow...\n-----END RSA PRIVATE KEY-----"),
	}
	c := &grpcClient{health: fake, timeout: time.Second}

	_, err := c.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck: expected error, got nil")
	}
	if strings.Contains(err.Error(), "BEGIN") || strings.Contains(err.Error(), "MIIEow") {
		t.Errorf("error leaks key material: %v", err)
	}
}

func TestSanitizeTLSErrorRedactsPEMBlocks(t *testing.T) {
	err := errors.New("some error: -----BEGIN CERTIFICATE-----\nsecretdata\n-----END CERTIFICATE-----")
	sanitized := sanitizeTLSError(err)

	if strings.Contains(sanitized.Error(), "secretdata") {
		t.Errorf("sanitizeTLSError did not redact PEM content: %v", sanitized)
	}
	if !strings.Contains(sanitized.Error(), "[REDACTED]") {
		t.Errorf("sanitizeTLSError did not add a redaction marker: %v", sanitized)
	}
}

func TestSanitizeTLSErrorNilIsNil(t *testing.T) {
	if sanitizeTLSError(nil) != nil {
		t.Error("sanitizeTLSError(nil) should return nil")
	}
}

// fakeClientSearcher implements clientSearcher without any real network
// or TLS setup, so grpcClient.SearchClients' logic can be tested
// directly.
type fakeClientSearcher struct {
	resp    *veloapi.SearchClientsResponse
	err     error
	delay   time.Duration
	sawReq  *veloapi.SearchClientsRequest
	sawCall bool
}

func (f *fakeClientSearcher) ListClients(ctx context.Context, in *veloapi.SearchClientsRequest, opts ...grpc.CallOption) (*veloapi.SearchClientsResponse, error) {
	f.sawCall = true
	f.sawReq = in
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return f.resp, f.err
}

func TestGRPCClientSearchClientsSuccess(t *testing.T) {
	fake := &fakeClientSearcher{
		resp: &veloapi.SearchClientsResponse{
			Items: []*veloapi.ApiClient{
				{
					ClientId:         "C.1234abcd5678ef90",
					LastIp:           "10.0.0.5",
					LastSeenAt:       1700000000000000,
					AgentInformation: &veloapi.AgentInformation{Version: "0.7.0"},
					OsInfo:           &veloapi.Uname{System: "windows", Hostname: "WIN-HOST"},
				},
			},
		},
	}
	c := &grpcClient{clients: fake, timeout: time.Second, maxRows: 50}

	clients, err := c.SearchClients(context.Background(), "WIN-HOST", 10)
	if err != nil {
		t.Fatalf("SearchClients: %v", err)
	}
	if len(clients) != 1 {
		t.Fatalf("len(clients) = %d, want 1", len(clients))
	}
	got := clients[0]
	if got.ClientID != "C.1234abcd5678ef90" || got.Hostname != "WIN-HOST" || got.OS != "windows" ||
		got.LastIP != "10.0.0.5" || got.AgentVersion != "0.7.0" || got.LastSeenAt == "" {
		t.Errorf("ClientSummary = %+v, missing expected fields", got)
	}
	if !fake.sawCall {
		t.Error("ListClients was never called")
	}
	if fake.sawReq.Query != "WIN-HOST" {
		t.Errorf("request Query = %q, want %q", fake.sawReq.Query, "WIN-HOST")
	}
	if fake.sawReq.Limit != 10 {
		t.Errorf("request Limit = %d, want 10", fake.sawReq.Limit)
	}
}

func TestGRPCClientSearchClientsBoundsLimitToMaxRows(t *testing.T) {
	fake := &fakeClientSearcher{resp: &veloapi.SearchClientsResponse{}}
	c := &grpcClient{clients: fake, timeout: time.Second, maxRows: 5}

	if _, err := c.SearchClients(context.Background(), "", 1000); err != nil {
		t.Fatalf("SearchClients: %v", err)
	}
	if fake.sawReq.Limit != 5 {
		t.Errorf("request Limit = %d, want it capped to maxRows=5", fake.sawReq.Limit)
	}
}

func TestGRPCClientSearchClientsTruncatesOversizedResponse(t *testing.T) {
	items := make([]*veloapi.ApiClient, 10)
	for i := range items {
		items[i] = &veloapi.ApiClient{ClientId: "C.1234abcd5678ef90"}
	}
	fake := &fakeClientSearcher{resp: &veloapi.SearchClientsResponse{Items: items}}
	c := &grpcClient{clients: fake, timeout: time.Second, maxRows: 100}

	clients, err := c.SearchClients(context.Background(), "", 3)
	if err != nil {
		t.Fatalf("SearchClients: %v", err)
	}
	if len(clients) != 3 {
		t.Errorf("len(clients) = %d, want 3 (a malicious/buggy server returning more rows than requested must still be truncated client-side)", len(clients))
	}
}

func TestGRPCClientSearchClientsError(t *testing.T) {
	fake := &fakeClientSearcher{err: errors.New("connection refused")}
	c := &grpcClient{clients: fake, timeout: time.Second}

	_, err := c.SearchClients(context.Background(), "x", 10)
	if err == nil {
		t.Fatal("SearchClients: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %v, want it to mention the underlying transport error", err)
	}
}

func TestGRPCClientSearchClientsTimeout(t *testing.T) {
	fake := &fakeClientSearcher{resp: &veloapi.SearchClientsResponse{}, delay: 50 * time.Millisecond}
	c := &grpcClient{clients: fake, timeout: 5 * time.Millisecond}

	start := time.Now()
	_, err := c.SearchClients(context.Background(), "x", 10)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("SearchClients: expected timeout error, got nil")
	}
	if elapsed > 40*time.Millisecond {
		t.Errorf("SearchClients took %v, expected it to respect the 5ms timeout", elapsed)
	}
}

func TestGRPCClientSearchClientsErrorDoesNotLeakSecrets(t *testing.T) {
	fake := &fakeClientSearcher{err: errors.New("tls: bad certificate: -----BEGIN RSA PRIVATE KEY-----\nMIIEow...\n-----END RSA PRIVATE KEY-----")}
	c := &grpcClient{clients: fake, timeout: time.Second}

	_, err := c.SearchClients(context.Background(), "x", 10)
	if err == nil {
		t.Fatal("SearchClients: expected error, got nil")
	}
	if strings.Contains(err.Error(), "BEGIN") {
		t.Errorf("error leaks key material: %v", err)
	}
}

// fakeClientGetter implements clientGetter without any real network or
// TLS setup, so grpcClient.GetClientInfo's logic can be tested directly.
type fakeClientGetter struct {
	resp *veloapi.ApiClient
	err  error
}

func (f *fakeClientGetter) GetClient(ctx context.Context, in *veloapi.GetClientRequest, opts ...grpc.CallOption) (*veloapi.ApiClient, error) {
	return f.resp, f.err
}

func TestGRPCClientGetClientInfoSuccess(t *testing.T) {
	fake := &fakeClientGetter{
		resp: &veloapi.ApiClient{
			ClientId: "C.1234abcd5678ef90",
			LastIp:   "10.0.0.5",
			Labels:   []string{"triage"},
			OsInfo:   &veloapi.Uname{System: "linux", Hostname: "srv1", MacAddresses: []string{"aa:bb:cc:dd:ee:ff"}},
		},
	}
	c := &grpcClient{clientDetail: fake, timeout: time.Second}

	detail, err := c.GetClientInfo(context.Background(), "C.1234abcd5678ef90")
	if err != nil {
		t.Fatalf("GetClientInfo: %v", err)
	}
	if detail.ClientID != "C.1234abcd5678ef90" || detail.Hostname != "srv1" {
		t.Errorf("ClientDetail = %+v, missing expected identity fields", detail)
	}
	if len(detail.Labels) != 1 || detail.Labels[0] != "triage" {
		t.Errorf("Labels = %v, want [triage]", detail.Labels)
	}
	if len(detail.MacAddresses) != 1 || detail.MacAddresses[0] != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("MacAddresses = %v, want [aa:bb:cc:dd:ee:ff]", detail.MacAddresses)
	}
}

// TestGRPCClientGetClientInfoNotFound covers the real-server behavior
// confirmed against a live Velociraptor lab (2026-07-06): GetClient
// returns a zero-value ApiClient, not an error, for an unknown client
// ID. GetClientInfo must translate that into ErrClientNotFound rather
// than a "successful" ClientDetail with every field blank.
func TestGRPCClientGetClientInfoNotFound(t *testing.T) {
	fake := &fakeClientGetter{resp: &veloapi.ApiClient{}}
	c := &grpcClient{clientDetail: fake, timeout: time.Second}

	_, err := c.GetClientInfo(context.Background(), "C.0000000000000000")
	if err == nil {
		t.Fatal("GetClientInfo: expected error, got nil")
	}
	if !errors.Is(err, ErrClientNotFound) {
		t.Errorf("GetClientInfo error = %v, want wrapping ErrClientNotFound", err)
	}
}

func TestGRPCClientGetClientInfoError(t *testing.T) {
	fake := &fakeClientGetter{err: errors.New("client not found")}
	c := &grpcClient{clientDetail: fake, timeout: time.Second}

	_, err := c.GetClientInfo(context.Background(), "C.1234abcd5678ef90")
	if err == nil {
		t.Fatal("GetClientInfo: expected error, got nil")
	}
}

func TestGRPCClientGetClientInfoErrorDoesNotLeakSecrets(t *testing.T) {
	fake := &fakeClientGetter{err: errors.New("tls: bad certificate: -----BEGIN CERTIFICATE-----\nsecret\n-----END CERTIFICATE-----")}
	c := &grpcClient{clientDetail: fake, timeout: time.Second}

	_, err := c.GetClientInfo(context.Background(), "C.1234abcd5678ef90")
	if err == nil {
		t.Fatal("GetClientInfo: expected error, got nil")
	}
	if strings.Contains(err.Error(), "BEGIN") || strings.Contains(err.Error(), "secret") {
		t.Errorf("error leaks certificate material: %v", err)
	}
}

// fakeArtifactCatalog implements artifactCatalog without any real
// network or TLS setup, so grpcClient's ListArtifactNames and
// GetArtifactDetails logic can be tested directly.
type fakeArtifactCatalog struct {
	resp   *veloapi.ArtifactDescriptors
	err    error
	sawReq *veloapi.GetArtifactsRequest
}

func (f *fakeArtifactCatalog) GetArtifacts(ctx context.Context, in *veloapi.GetArtifactsRequest, opts ...grpc.CallOption) (*veloapi.ArtifactDescriptors, error) {
	f.sawReq = in
	return f.resp, f.err
}

func TestGRPCClientListArtifactNamesSuccess(t *testing.T) {
	fake := &fakeArtifactCatalog{
		resp: &veloapi.ArtifactDescriptors{
			Items: []*veloapi.Artifact{
				{Name: "Generic.Client.Info", Description: "Basic client info"},
				{Name: "Windows.System.Pslist", Description: "Process list"},
			},
		},
	}
	c := &grpcClient{artifacts: fake, timeout: time.Second, maxRows: 50}

	names, err := c.ListArtifactNames(context.Background())
	if err != nil {
		t.Fatalf("ListArtifactNames: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("len(names) = %d, want 2", len(names))
	}
	if names[0].Name != "Generic.Client.Info" || names[0].Description != "Basic client info" {
		t.Errorf("names[0] = %+v, unexpected", names[0])
	}
	if fake.sawReq.NumberOfResults != 50 {
		t.Errorf("NumberOfResults = %d, want 50 (bounded by maxRows)", fake.sawReq.NumberOfResults)
	}
}

func TestGRPCClientListArtifactNamesError(t *testing.T) {
	fake := &fakeArtifactCatalog{err: errors.New("connection refused")}
	c := &grpcClient{artifacts: fake, timeout: time.Second}

	_, err := c.ListArtifactNames(context.Background())
	if err == nil {
		t.Fatal("ListArtifactNames: expected error, got nil")
	}
}

func TestGRPCClientGetArtifactDetailsSuccess(t *testing.T) {
	fake := &fakeArtifactCatalog{
		resp: &veloapi.ArtifactDescriptors{
			Items: []*veloapi.Artifact{
				{
					Name:        "Windows.System.Pslist",
					Description: "Process list",
					Parameters: []*veloapi.ArtifactParameter{
						{Name: "Pid", Type: "int", Description: "process id", Default: "0"},
					},
				},
			},
		},
	}
	c := &grpcClient{artifacts: fake, timeout: time.Second}

	detail, err := c.GetArtifactDetails(context.Background(), "Windows.System.Pslist")
	if err != nil {
		t.Fatalf("GetArtifactDetails: %v", err)
	}
	if detail.Name != "Windows.System.Pslist" {
		t.Errorf("Name = %q, want %q", detail.Name, "Windows.System.Pslist")
	}
	if len(detail.Parameters) != 1 || detail.Parameters[0].Name != "Pid" {
		t.Errorf("Parameters = %+v, unexpected", detail.Parameters)
	}
	if fake.sawReq.Names[0] != "Windows.System.Pslist" {
		t.Errorf("request Names = %v, want [Windows.System.Pslist]", fake.sawReq.Names)
	}
}

func TestGRPCClientGetArtifactDetailsNotFound(t *testing.T) {
	fake := &fakeArtifactCatalog{resp: &veloapi.ArtifactDescriptors{}}
	c := &grpcClient{artifacts: fake, timeout: time.Second}

	_, err := c.GetArtifactDetails(context.Background(), "Does.Not.Exist")
	if err == nil {
		t.Fatal("GetArtifactDetails: expected error for unknown artifact, got nil")
	}
	if !errors.Is(err, ErrArtifactNotFound) {
		t.Errorf("GetArtifactDetails error = %v, want wrapping ErrArtifactNotFound", err)
	}
}

func TestGRPCClientGetArtifactDetailsErrorDoesNotLeakSecrets(t *testing.T) {
	fake := &fakeArtifactCatalog{err: errors.New("tls: bad certificate: -----BEGIN RSA PRIVATE KEY-----\nsecret\n-----END RSA PRIVATE KEY-----")}
	c := &grpcClient{artifacts: fake, timeout: time.Second}

	_, err := c.GetArtifactDetails(context.Background(), "Windows.System.Pslist")
	if err == nil {
		t.Fatal("GetArtifactDetails: expected error, got nil")
	}
	if strings.Contains(err.Error(), "BEGIN") || strings.Contains(err.Error(), "secret") {
		t.Errorf("error leaks key material: %v", err)
	}
}
