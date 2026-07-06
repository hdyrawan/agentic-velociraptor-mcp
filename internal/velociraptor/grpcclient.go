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

// defaultMaxRows bounds visibility-tool result sizes when grpcClient was
// constructed with a non-positive maxRows (config.VelociraptorConfig.MaxRows
// left unset or invalid). The ceiling must never be zero/unbounded: an
// unconfigured limit fails closed to a small number rather than asking
// Velociraptor for "everything".
const defaultMaxRows = 100

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

// clientSearcher is the narrow seam SearchClients goes through.
// veloapi.APIClient satisfies it.
type clientSearcher interface {
	ListClients(ctx context.Context, in *veloapi.SearchClientsRequest, opts ...grpc.CallOption) (*veloapi.SearchClientsResponse, error)
}

// clientGetter is the narrow seam GetClientInfo goes through.
// veloapi.APIClient satisfies it.
type clientGetter interface {
	GetClient(ctx context.Context, in *veloapi.GetClientRequest, opts ...grpc.CallOption) (*veloapi.ApiClient, error)
}

// artifactCatalog is the narrow seam ListArtifactNames and
// GetArtifactDetails go through. veloapi.APIClient satisfies it.
type artifactCatalog interface {
	GetArtifacts(ctx context.Context, in *veloapi.GetArtifactsRequest, opts ...grpc.CallOption) (*veloapi.ArtifactDescriptors, error)
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

	health       healthChecker
	clients      clientSearcher
	clientDetail clientGetter
	artifacts    artifactCatalog

	timeout time.Duration
	orgID   string

	// maxRows bounds every result list this client returns
	// (config.VelociraptorConfig.MaxRows), independent of and in
	// addition to whatever limit a caller requests. See effectiveMaxRows
	// and boundLimit.
	maxRows int
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
// (config.VelociraptorConfig.TimeoutSeconds). maxRows bounds every
// result list this client returns (config.VelociraptorConfig.MaxRows);
// a non-positive value falls back to defaultMaxRows rather than being
// treated as "unbounded".
func NewGRPCClient(apiConfigPath, orgID string, timeout time.Duration, maxRows int) (Client, error) {
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

	apiClient := veloapi.NewAPIClient(conn)
	return &grpcClient{
		health:       apiClient,
		clients:      apiClient,
		clientDetail: apiClient,
		artifacts:    apiClient,
		timeout:      timeout,
		orgID:        orgID,
		maxRows:      maxRows,
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

// effectiveMaxRows returns c.maxRows if positive, else defaultMaxRows.
func (c *grpcClient) effectiveMaxRows() int {
	if c.maxRows > 0 {
		return c.maxRows
	}
	return defaultMaxRows
}

// boundLimit clamps a caller-requested limit to (0, ceiling]: a
// non-positive or overlarge request is silently capped rather than
// rejected, since "return at most N rows" is inherently advisory on the
// caller's side and must still be enforced server-response-side
// regardless of what was asked for.
func boundLimit(requested, ceiling int) int {
	if ceiling <= 0 {
		ceiling = defaultMaxRows
	}
	if requested <= 0 || requested > ceiling {
		return ceiling
	}
	return requested
}

// microsecondsToRFC3339 formats a Velociraptor timestamp (microseconds
// since the Unix epoch, as used by ApiClient.last_seen_at) as an RFC3339
// string, or "" if ts is zero (a client that has genuinely never
// checked in, as opposed to a real epoch-zero timestamp).
func microsecondsToRFC3339(ts uint64) string {
	if ts == 0 {
		return ""
	}
	return time.UnixMicro(int64(ts)).UTC().Format(time.RFC3339)
}

// toClientSummary maps a Velociraptor ApiClient onto the safe,
// minimal ClientSummary shape velo_search_clients and
// velo_get_client_info return. Every field here is server-observed
// endpoint metadata; see ClientReader's doc comment on not overstating
// trust in client-reported data.
func toClientSummary(c *veloapi.ApiClient) ClientSummary {
	summary := ClientSummary{
		ClientID:   c.GetClientId(),
		LastIP:     c.GetLastIp(),
		LastSeenAt: microsecondsToRFC3339(c.GetLastSeenAt()),
	}
	if osInfo := c.GetOsInfo(); osInfo != nil {
		summary.Hostname = osInfo.GetHostname()
		summary.OS = osInfo.GetSystem()
	}
	if agent := c.GetAgentInformation(); agent != nil {
		summary.AgentVersion = agent.GetVersion()
	}
	return summary
}

// SearchClients calls Velociraptor's own ListClients RPC (the same RPC
// backing the GUI's client search box) with query bound as a plain
// SearchClientsRequest.Query field — Velociraptor's client search
// syntax, never VQL, and never string-concatenated into anything. See
// internal/vql's package doc for why this project treats "bound as a
// protobuf field" and "interpolated into a query string" as
// categorically different things.
func (c *grpcClient) SearchClients(ctx context.Context, query string, limit int) ([]ClientSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	bounded := boundLimit(limit, c.effectiveMaxRows())

	resp, err := c.clients.ListClients(ctx, &veloapi.SearchClientsRequest{
		Query: query,
		Limit: uint64(bounded),
	})
	if err != nil {
		return nil, fmt.Errorf("velociraptor: search clients: %w", sanitizeTLSError(err))
	}

	items := resp.GetItems()
	if len(items) > bounded {
		items = items[:bounded]
	}

	out := make([]ClientSummary, 0, len(items))
	for _, item := range items {
		out = append(out, toClientSummary(item))
	}
	return out, nil
}

// GetClientInfo calls Velociraptor's GetClient RPC for one
// already-validated client ID and returns its safe endpoint metadata,
// including the fields ClientSummary omits (labels, MAC addresses).
//
// Confirmed against a real Velociraptor lab server: GetClient does not
// return an error for an unknown client ID — it returns a zero-value
// ApiClient (empty ClientId and every other field) instead. Without the
// check below, that would surface as a "successful" velo_get_client_info
// result carrying a client record with an empty client_id, which reads
// as a real (if sparse) client rather than "no such client" — a
// violation of this project's evidence-honesty principle (see
// docs/security-model.md). ErrClientNotFound lets the handler report
// this the same way it reports any other real-mode failure.
func (c *grpcClient) GetClientInfo(ctx context.Context, clientID string) (ClientDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.clientDetail.GetClient(ctx, &veloapi.GetClientRequest{ClientId: clientID})
	if err != nil {
		return ClientDetail{}, fmt.Errorf("velociraptor: get client info: %w", sanitizeTLSError(err))
	}
	if resp.GetClientId() == "" {
		return ClientDetail{}, fmt.Errorf("velociraptor: get client info: %w", ErrClientNotFound)
	}

	detail := ClientDetail{
		ClientSummary: toClientSummary(resp),
		Labels:        resp.GetLabels(),
	}
	if osInfo := resp.GetOsInfo(); osInfo != nil {
		detail.MacAddresses = osInfo.GetMacAddresses()
	}
	return detail, nil
}

// toArtifactParameters maps Velociraptor ArtifactParameter messages onto
// this project's ArtifactParameter shape, never touching any field
// beyond name/type/description/default.
func toArtifactParameters(params []*veloapi.ArtifactParameter) []ArtifactParameter {
	if len(params) == 0 {
		return nil
	}
	out := make([]ArtifactParameter, 0, len(params))
	for _, p := range params {
		out = append(out, ArtifactParameter{
			Name:        p.GetName(),
			Type:        p.GetType(),
			Description: p.GetDescription(),
			Default:     p.GetDefault(),
		})
	}
	return out
}

// ListArtifactNames calls Velociraptor's GetArtifacts RPC with no name
// filter, bounded by NumberOfResults, and returns only name/description
// per artifact — never the ArtifactSource entries the same server
// response carries (those contain VQL query bodies; see
// internal/velociraptor/veloapi/visibility.proto's Artifact message,
// which has no field to decode them into).
func (c *grpcClient) ListArtifactNames(ctx context.Context) ([]ArtifactSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	bounded := c.effectiveMaxRows()

	resp, err := c.artifacts.GetArtifacts(ctx, &veloapi.GetArtifactsRequest{
		NumberOfResults: uint64(bounded),
	})
	if err != nil {
		return nil, fmt.Errorf("velociraptor: list artifact names: %w", sanitizeTLSError(err))
	}

	items := resp.GetItems()
	if len(items) > bounded {
		items = items[:bounded]
	}

	out := make([]ArtifactSummary, 0, len(items))
	for _, a := range items {
		out = append(out, ArtifactSummary{Name: a.GetName(), Description: a.GetDescription()})
	}
	return out, nil
}

// GetArtifactDetails calls Velociraptor's GetArtifacts RPC filtered to
// exactly one already-validated artifact name and returns its parameter
// schema — never its VQL body (see ListArtifactNames's doc comment).
func (c *grpcClient) GetArtifactDetails(ctx context.Context, name string) (ArtifactDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.artifacts.GetArtifacts(ctx, &veloapi.GetArtifactsRequest{
		Names: []string{name},
	})
	if err != nil {
		return ArtifactDetail{}, fmt.Errorf("velociraptor: get artifact details: %w", sanitizeTLSError(err))
	}

	items := resp.GetItems()
	if len(items) == 0 {
		return ArtifactDetail{}, fmt.Errorf("velociraptor: artifact %q not found: %w", name, ErrArtifactNotFound)
	}

	a := items[0]
	return ArtifactDetail{
		ArtifactSummary: ArtifactSummary{Name: a.GetName(), Description: a.GetDescription()},
		Parameters:      toArtifactParameters(a.GetParameters()),
	}, nil
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
