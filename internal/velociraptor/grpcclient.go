package velociraptor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor/veloapi"
)

// defaultPinnedServerName is the TLS ServerName Velociraptor's own
// clients present when an api.config.yaml doesn't specify
// pinned_server_name, matching the upstream project's
// constants.PinnedServerName. Velociraptor's self-signed server
// certificates are issued for this fixed name rather than a real DNS
// hostname, so TLS verification pins against it explicitly instead of
// (or as well as) the connection address.
const defaultPinnedServerName = "VelociraptorServer"

// healthChecker is the narrow seam real health checking goes through.
// veloapi.APIClient satisfies it. Tests substitute a fake implementing
// just this method, so HealthCheck's timeout/error-handling logic can be
// exercised without any real TLS handshake or network connection.
type healthChecker interface {
	Check(ctx context.Context, in *veloapi.HealthCheckRequest, opts ...grpc.CallOption) (*veloapi.HealthCheckResponse, error)
}

// grpcClient is a real, mTLS-authenticated Velociraptor gRPC client.
//
// As of v0.1.0-alpha.2 it implements only a real HealthCheck; every
// other Client method is delegated to the embedded placeholderClient
// (still ErrNotImplemented). No collection, hunt, flow, or upload
// capability exists yet — see PROJECT_PLAN.md's v0.1.0-alpha.2 scope and
// docs/security-model.md for why those remain out of scope in this
// milestone. grpcClient is only ever constructed from
// config.VelociraptorConfig.ReadAPIConfigPath; nothing in this file
// reads or uses WriteAPIConfigPath.
type grpcClient struct {
	placeholderClient

	health  healthChecker
	timeout time.Duration
	orgID   string
}

// NewGRPCClient loads apiConfigPath (a Velociraptor api.config.yaml),
// builds an mTLS gRPC connection to the server it describes, and
// returns a Client backed by it. It does not perform any network I/O
// itself — grpc.NewClient connects lazily on first RPC — so a
// misconfigured or unreachable server is only discovered when
// HealthCheck (or a future real RPC) is actually called, and is always
// reported through that call's return value rather than by this
// constructor succeeding or failing.
//
// timeout bounds every individual RPC made through the returned client
// (config.VelociraptorConfig.TimeoutSeconds).
func NewGRPCClient(apiConfigPath, orgID string, timeout time.Duration) (Client, error) {
	apiCfg, err := LoadAPIConfig(apiConfigPath)
	if err != nil {
		return nil, err
	}

	cert, err := tls.X509KeyPair([]byte(apiCfg.ClientCert), []byte(apiCfg.ClientPrivateKey))
	if err != nil {
		return nil, fmt.Errorf("velociraptor: parse client certificate/private key: %w", sanitizeTLSError(err))
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM([]byte(apiCfg.CACertificate)) {
		return nil, fmt.Errorf("velociraptor: parse CA certificate: invalid PEM data")
	}

	serverName := apiCfg.PinnedServerName
	if serverName == "" {
		serverName = defaultPinnedServerName
	}

	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS12,
	})

	conn, err := grpc.NewClient(apiCfg.APIConnectionString, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("velociraptor: create gRPC client for %s: %w", apiCfg.APIConnectionString, sanitizeTLSError(err))
	}

	return &grpcClient{
		health:  veloapi.NewAPIClient(conn),
		timeout: timeout,
		orgID:   orgID,
	}, nil
}

// HealthCheck calls the Velociraptor API server's dedicated Check RPC
// (modeled on the standard gRPC health-checking protocol; see
// internal/velociraptor/veloapi/health.proto), bounded by c.timeout.
//
// Deliberately not implemented via a VQL query (e.g. `SELECT * FROM
// info()`): Check is the real, documented, purpose-built health-check
// RPC in Velociraptor's own API service and requires no query
// construction, parameter binding, or result parsing at all — it is
// strictly narrower and safer than standing up any part of the VQL
// query path, which remains entirely out of scope for the stable core
// (see docs/security-model.md). One consequence: Check's response
// carries no server version string, so Info.ServerVersion is always
// empty for a real client in this milestone.
func (c *grpcClient) HealthCheck(ctx context.Context) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.health.Check(ctx, &veloapi.HealthCheckRequest{})
	if err != nil {
		return Info{}, fmt.Errorf("velociraptor: health check: %w", sanitizeTLSError(err))
	}
	if resp.GetStatus() != veloapi.HealthCheckResponse_SERVING {
		return Info{}, fmt.Errorf("velociraptor: health check: server reported status %s", resp.GetStatus())
	}

	return Info{OrgID: c.orgID}, nil
}

// sanitizeTLSError is a defense-in-depth guard: TLS/x509/gRPC error
// strings from the standard library and google.golang.org/grpc never
// legitimately embed PEM key material, but this strips anything
// PEM-shaped from an error's text anyway, on the theory that a health
// check error is exactly the kind of message that tends to get pasted
// into a ticket or chat without a second thought.
func sanitizeTLSError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if idx := strings.Index(msg, "-----BEGIN"); idx != -1 {
		msg = msg[:idx] + "[REDACTED]"
	}
	return fmt.Errorf("%s", msg)
}
