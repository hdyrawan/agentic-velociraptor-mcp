// Package velociraptor is the only package permitted to talk to a
// Velociraptor server. It wraps the Velociraptor gRPC API (never the
// internal REST API) behind narrow, typed methods; there is no
// "run arbitrary VQL" method on Client, by design.
//
// TODO(v0.1.0-alpha.2): implement a real gRPC-backed Client using
// Velociraptor's api.proto client bindings, authenticated via mTLS from
// a loaded api.config.yaml (read or write identity per
// config.VelociraptorConfig). Until then, NewClient returns a client
// that fails every call, so callers written against this interface can
// be exercised in tests without a live server.
package velociraptor

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned by every method of the placeholder
// client shipped in v0.0.x.
var ErrNotImplemented = errors.New("velociraptor: not implemented in this build")

// Client is the full set of Velociraptor operations the MCP server may
// perform, split by concern across clients.go, artifacts.go, flows.go,
// hunts.go, and uploads.go. Each sub-interface corresponds to one group
// of MCP tools.
//
// Deliberately absent: any method that executes caller-supplied VQL
// text, and any method that grants filesystem write or process execution
// on the Velociraptor server itself.
type Client interface {
	HealthChecker
	ClientReader
	ArtifactReader
	FlowReader
	FlowWriter
	HuntReader
	HuntWriter
	UploadReader
	UploadDownloader
}

// HealthChecker verifies connectivity and identity against the
// Velociraptor gRPC API without touching any client/endpoint state.
type HealthChecker interface {
	HealthCheck(ctx context.Context) (Info, error)
}

// Info is the minimal, non-sensitive server identity info returned by a
// health check.
type Info struct {
	ServerVersion string
	OrgID         string
}

// placeholderClient implements Client and returns ErrNotImplemented for
// every operation. It exists so the rest of the codebase (policy,
// mcpserver placeholders, tests) can depend on the Client interface
// today.
type placeholderClient struct{}

// NewClient returns the current placeholder Client. cfg is unused for
// now; its shape documents what a real constructor will need.
//
// TODO(v0.1.0-alpha.2): accept a resolved config.VelociraptorConfig (or
// a narrower struct with just the API config path + org ID + timeout),
// load the referenced api.config.yaml, establish an mTLS gRPC
// connection, and return a client backed by it. NewClient must never log
// the contents of cfg.
func NewClient() Client {
	return placeholderClient{}
}

func (placeholderClient) HealthCheck(ctx context.Context) (Info, error) {
	return Info{}, ErrNotImplemented
}
