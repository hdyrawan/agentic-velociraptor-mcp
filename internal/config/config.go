// Package config defines the on-disk configuration model for
// agentic-velociraptor-mcp and loads it from YAML.
//
// The configuration is deliberately conservative: policy defaults to
// read-only, raw VQL is disabled, and risky operation categories require
// explicit approval. See docs/security-model.md and docs/configuration.md.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration object loaded from the server's YAML
// config file.
type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Velociraptor VelociraptorConfig `yaml:"velociraptor"`
	Policy       PolicyConfig       `yaml:"policy"`
	Audit        AuditConfig        `yaml:"audit"`
	Approval     ApprovalConfig     `yaml:"approval"`
}

// ServerConfig describes MCP server identity and transport.
type ServerConfig struct {
	Name string `yaml:"name"`

	// Transport selects the MCP transport. Only "stdio" is supported in
	// the initial releases. HTTP/SSE/streamable transports are explicitly
	// out of scope until requested; see PROJECT_PLAN.md.
	Transport string `yaml:"transport"`
}

// VelociraptorConfig describes how to reach the Velociraptor gRPC API and
// the resource limits applied to every call made through it.
//
// Two separate API config paths are supported intentionally: a read-only
// identity for visibility tools, and a separate, more privileged (but
// still least-privilege) identity for write/collection/hunt operations.
// Never merge these into a single "do everything" identity.
type VelociraptorConfig struct {
	OrgID string `yaml:"org_id"`

	// ReadAPIConfigPath points at a Velociraptor api.config.yaml (mTLS
	// client cert bundle) for an identity that holds only read-oriented
	// ACLs (e.g. READ_RESULTS, COLLECT_CLIENT for allowlisted artifacts).
	// This path is treated as a secret; its contents must never be
	// logged. See docs/velociraptor-permissions.md.
	ReadAPIConfigPath string `yaml:"read_api_config_path"`

	// WriteAPIConfigPath points at a separate api.config.yaml for an
	// identity used only for approved, write-capable operations
	// (collections, hunts, cancellations, downloads). Must NOT hold
	// administrator, ARTIFACT_WRITER, SERVER_ARTIFACT_WRITER, EXECVE,
	// FILESYSTEM_WRITE, or SERVER_ADMIN.
	WriteAPIConfigPath string `yaml:"write_api_config_path"`

	TimeoutSeconds int   `yaml:"timeout_seconds"`
	MaxRows        int   `yaml:"max_rows"`
	MaxResultBytes int64 `yaml:"max_result_bytes"`
	MaxUploadBytes int64 `yaml:"max_upload_bytes"`

	// DownloadDir is a local directory velo_download_flow_upload_with_approval
	// writes downloaded evidence bytes to. Left empty by default, which
	// disables that tool outright (see mcpserver.writePilotEnabled):
	// evidence disclosure must be explicitly opted into by an operator
	// pointing this at a real, access-controlled directory, never
	// enabled implicitly by policy.mode alone. The tool never returns
	// raw evidence bytes inline in an MCP response; only this local
	// path, size, and a checksum.
	DownloadDir string `yaml:"download_dir"`
}

// PolicyMode selects the overall operating posture of the MCP-layer
// policy engine. This is a defense-in-depth control layered on top of
// Velociraptor-native ACLs, not a substitute for them.
type PolicyMode string

const (
	// PolicyModeReadOnly disables every tool that can mutate endpoint or
	// server state, regardless of RequireApprovalFor contents. This is
	// the default and recommended mode for v0.1.0.
	PolicyModeReadOnly PolicyMode = "read_only"

	// PolicyModeControlled allows write-capable tools, gated by
	// RequireApprovalFor and the allowlists below.
	PolicyModeControlled PolicyMode = "controlled"
)

// PolicyConfig captures the MCP-layer policy: what categories of
// operation exist, which require human approval, and which artifacts,
// profiles, and targeting modes are allowed at all.
//
// TODO(v0.1.0+): wire PolicyConfig into internal/policy.Engine once real
// tool handlers exist. For the v0.0.x skeleton this only defines shape.
type PolicyConfig struct {
	Mode PolicyMode `yaml:"mode"`

	// AllowRawVQL must remain false in every shipped default. Raw VQL is
	// not exposed as a tool in the stable core; this flag exists so the
	// restriction is explicit and auditable in config, not implied by
	// omission.
	AllowRawVQL bool `yaml:"allow_raw_vql"`

	// AllowListAllArtifacts controls whether velo_list_artifact_names may
	// return the full server artifact catalog rather than only the
	// configured AllowedArtifacts. Even when true, artifact *use* is
	// still constrained by AllowedArtifacts.
	AllowListAllArtifacts bool `yaml:"allow_list_all_artifacts"`

	// AllowTargetAll must stay false by default. When false, hunts and
	// collections cannot target "all clients" scopes.
	AllowTargetAll bool `yaml:"allow_target_all"`

	MaxHuntClients int `yaml:"max_hunt_clients"`

	// RequireApprovalFor lists operation categories (matching
	// approval.Operation values) that must go through the approval
	// workflow before execution, even in Controlled mode.
	RequireApprovalFor []string `yaml:"require_approval_for"`

	AllowedArtifacts []string `yaml:"allowed_artifacts"`
	AllowedProfiles  []string `yaml:"allowed_profiles"`
}

// AuditConfig controls the JSONL audit log.
type AuditConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`

	// RedactFields lists structured field names that must never appear
	// in plaintext in an audit record. internal/audit/sanitize.go is the
	// single choke point responsible for enforcing this list.
	RedactFields []string `yaml:"redact_fields"`
}

// ApprovalConfig points at the on-disk approval.Store used by every
// write-capable, approval-gated tool (collection, DFIR profile
// collection, flow cancellation, upload download).
//
// StorePath is left empty by default. An empty StorePath is one of the
// two conditions (alongside Policy.Mode) that must both be explicitly
// set before any approval-gated tool will do anything but report itself
// disabled; see mcpserver.writePilotEnabled. The store file itself is
// created and updated only by the agentic-velociraptor-mcp `approve` CLI
// subcommand and by internal/approval.FileStore's Consume calls from
// tool handlers — never written to directly by an MCP tool's Create/
// Decide path, since no MCP tool calls those.
type ApprovalConfig struct {
	StorePath string `yaml:"store_path"`

	// TTLSeconds bounds how long an approved-but-unused approval remains
	// usable, measured from the Request's creation time. Must be > 0
	// whenever StorePath is set.
	TTLSeconds int `yaml:"ttl_seconds"`
}

// Load reads and parses a YAML config file at path. It does not apply
// defaults or validate semantic constraints; call Validate on the result.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	return &cfg, nil
}

// Default returns a conservative, read-only default configuration. It is
// intentionally unusable as-is (empty API config paths) and exists as a
// documented starting point and for tests.
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Name:      "agentic-velociraptor-mcp",
			Transport: "stdio",
		},
		Velociraptor: VelociraptorConfig{
			OrgID:          "root",
			TimeoutSeconds: 30,
			MaxRows:        500,
			MaxResultBytes: 1048576,
			MaxUploadBytes: 52428800,
			DownloadDir:    "",
		},
		Policy: PolicyConfig{
			Mode:                  PolicyModeReadOnly,
			AllowRawVQL:           false,
			AllowListAllArtifacts: false,
			AllowTargetAll:        false,
			MaxHuntClients:        100,
			RequireApprovalFor: []string{
				"collect_artifact",
				"collect_dfir_profile",
				"start_hunt",
				"start_dfir_hunt",
				"cancel_flow",
				"cancel_hunt",
				"download_flow_upload",
				"hunt_ioc",
			},
			AllowedArtifacts: []string{
				"Generic.Client.Info",
				"Windows.System.Pslist",
				"Windows.Network.Netstat",
			},
			AllowedProfiles: []string{
				"windows_basic_triage",
				"windows_ransomware_triage",
				"linux_basic_triage",
			},
		},
		Audit: AuditConfig{
			Enabled: true,
			Path:    "/var/log/agentic-velociraptor-mcp/audit.jsonl",
			RedactFields: []string{
				"client_private_key",
				"client_cert",
				"ca_certificate",
				"api_key",
				"approval_token",
				"password",
				"secret",
			},
		},
		Approval: ApprovalConfig{
			StorePath:  "",
			TTLSeconds: 900,
		},
	}
}
