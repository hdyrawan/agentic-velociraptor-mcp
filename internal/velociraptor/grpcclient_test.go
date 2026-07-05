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
