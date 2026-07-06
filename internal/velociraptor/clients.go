package velociraptor

import (
	"context"
	"errors"
)

// ErrClientNotFound is returned by GetClientInfo when Velociraptor has
// no record of the requested client ID. Velociraptor's real GetClient
// RPC does not error in this case — it returns a zero-value ApiClient —
// so implementations must detect and translate that themselves (see
// grpcClient.GetClientInfo); this sentinel gives callers (and their
// audit/error-message handling) a distinguishable, honest outcome
// instead of a client record with every field blank.
var ErrClientNotFound = errors.New("velociraptor: client not found")

// ClientSummary is the subset of Velociraptor client fields exposed by
// velo_search_clients: enough to identify and triage-select an endpoint,
// nothing beyond that (no raw client metadata blobs).
type ClientSummary struct {
	ClientID     string
	Hostname     string
	OS           string
	LastSeenAt   string
	LastIP       string
	AgentVersion string
}

// ClientDetail is the fuller record returned by velo_get_client_info for
// a single, already-identified client.
type ClientDetail struct {
	ClientSummary
	Labels       []string
	MacAddresses []string
}

// ClientReader backs velo_search_clients and velo_get_client_info.
type ClientReader interface {
	// SearchClients looks up clients matching a query (hostname/IP/label
	// substring, not raw VQL). query is validated and passed as a safe
	// query parameter by the implementation, never string-concatenated
	// into VQL; see internal/vql.
	SearchClients(ctx context.Context, query string, limit int) ([]ClientSummary, error)

	// GetClientInfo returns detail for one client, already validated as
	// a well-formed client ID by the caller (internal/validation).
	GetClientInfo(ctx context.Context, clientID string) (ClientDetail, error)
}

func (placeholderClient) SearchClients(ctx context.Context, query string, limit int) ([]ClientSummary, error) {
	return nil, ErrNotImplemented
}

func (placeholderClient) GetClientInfo(ctx context.Context, clientID string) (ClientDetail, error) {
	return ClientDetail{}, ErrNotImplemented
}
