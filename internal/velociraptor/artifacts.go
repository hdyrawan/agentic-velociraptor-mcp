package velociraptor

import (
	"context"
	"errors"
)

// ErrArtifactNotFound is returned by GetArtifactDetails when Velociraptor
// has no artifact matching the requested name. Mirrors ErrClientNotFound
// (see clients.go): a distinguishable sentinel instead of a bare
// fmt.Errorf lets callers (and their audit/response-status handling)
// tell "no such artifact" apart from a connectivity or transport
// failure.
var ErrArtifactNotFound = errors.New("velociraptor: artifact not found")

// ArtifactSummary is a minimal artifact catalog entry.
type ArtifactSummary struct {
	Name        string
	Description string
}

// ArtifactDetail includes the parameter schema an artifact accepts, so
// callers (and the agent) can see what a collection would require
// without ever seeing the artifact's VQL body. Whether to also expose
// the VQL body itself is an open design question; the current default
// is not to, to avoid nudging agents toward reasoning about raw VQL. See
// docs/security-model.md.
type ArtifactDetail struct {
	ArtifactSummary
	Parameters []ArtifactParameter
}

// ArtifactParameter describes one artifact-level parameter.
type ArtifactParameter struct {
	Name        string
	Type        string
	Description string
	Default     string
}

// ArtifactReader backs velo_list_artifact_names and
// velo_get_artifact_details. Both operate only over the configured
// allowlist unless config.PolicyConfig.AllowListAllArtifacts is true, in
// which case listing (not collection) may show the full catalog.
type ArtifactReader interface {
	ListArtifactNames(ctx context.Context) ([]ArtifactSummary, error)
	GetArtifactDetails(ctx context.Context, name string) (ArtifactDetail, error)
}

func (placeholderClient) ListArtifactNames(ctx context.Context) ([]ArtifactSummary, error) {
	return nil, ErrNotImplemented
}

func (placeholderClient) GetArtifactDetails(ctx context.Context, name string) (ArtifactDetail, error) {
	return ArtifactDetail{}, ErrNotImplemented
}
