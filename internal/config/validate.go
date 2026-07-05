package config

import (
	"errors"
	"fmt"
)

// Validate applies basic structural sanity checks to a loaded Config. It
// deliberately does not try to be exhaustive; deeper semantic validation
// (e.g. artifact name syntax, profile allowlist cross-checks) belongs in
// the packages that own those concerns (internal/validation,
// internal/dfir) and should be layered on top of this.
func Validate(cfg *Config) error {
	if cfg == nil {
		return errors.New("config: nil config")
	}

	var errs []error

	if cfg.Server.Transport != "stdio" {
		errs = append(errs, fmt.Errorf("server.transport: only %q is supported, got %q", "stdio", cfg.Server.Transport))
	}

	// ReadAPIConfigPath is intentionally optional as of v0.1.0-alpha.2:
	// when empty, velo_health_check runs in mock mode rather than
	// attempting a real Velociraptor gRPC connection (see
	// cmd/agentic-velociraptor-mcp's buildDeps and
	// internal/mcpserver/tools_visibility.go). When non-empty, the path
	// must actually load as a valid Velociraptor api.config.yaml — that
	// check happens at startup in buildDeps via
	// internal/velociraptor.NewGRPCClient, not here, since it requires
	// reading and parsing the file rather than a structural check on
	// Config alone.
	//
	// WriteAPIConfigPath is likewise optional: a deployment may run
	// read-only forever, and no code path uses it as of
	// v0.1.0-alpha.2. It becomes required only when Policy.Mode is
	// Controlled and a write-capable tool is actually invoked; that check
	// belongs to the tool handler, not global validation.

	if cfg.Velociraptor.TimeoutSeconds <= 0 {
		errs = append(errs, errors.New("velociraptor.timeout_seconds must be > 0"))
	}
	if cfg.Velociraptor.MaxRows <= 0 {
		errs = append(errs, errors.New("velociraptor.max_rows must be > 0"))
	}
	if cfg.Velociraptor.MaxResultBytes <= 0 {
		errs = append(errs, errors.New("velociraptor.max_result_bytes must be > 0"))
	}
	if cfg.Velociraptor.MaxUploadBytes <= 0 {
		errs = append(errs, errors.New("velociraptor.max_upload_bytes must be > 0"))
	}

	switch cfg.Policy.Mode {
	case PolicyModeReadOnly, PolicyModeControlled:
	default:
		errs = append(errs, fmt.Errorf("policy.mode: unknown mode %q", cfg.Policy.Mode))
	}

	if cfg.Policy.AllowRawVQL {
		errs = append(errs, errors.New("policy.allow_raw_vql: raw VQL is not supported by any released tool; this flag must stay false"))
	}

	if cfg.Policy.MaxHuntClients <= 0 {
		errs = append(errs, errors.New("policy.max_hunt_clients must be > 0"))
	}

	if cfg.Audit.Enabled && cfg.Audit.Path == "" {
		errs = append(errs, errors.New("audit.path is required when audit.enabled is true"))
	}

	return errors.Join(errs...)
}
