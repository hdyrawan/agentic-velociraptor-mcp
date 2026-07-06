package config

import (
	"os"
	"testing"
)

func TestDefaultPassesValidate(t *testing.T) {
	cfg := Default()

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate(Default()) = %v, want nil", err)
	}
}

func TestValidateAllowsEmptyReadAPIConfigPath(t *testing.T) {
	// As of v0.1.0-alpha.2, an empty read_api_config_path is a valid,
	// supported configuration: it means velo_health_check runs in mock
	// mode rather than attempting a real Velociraptor connection. See
	// cmd/agentic-velociraptor-mcp's buildDeps.
	cfg := Default()
	cfg.Velociraptor.ReadAPIConfigPath = ""

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate: expected nil for empty read_api_config_path, got %v", err)
	}
}

func TestValidateRejectsRawVQL(t *testing.T) {
	cfg := Default()
	cfg.Policy.AllowRawVQL = true

	if err := Validate(cfg); err == nil {
		t.Fatal("Validate: expected error when allow_raw_vql is true, got nil")
	}
}

func TestValidateAllowsEmptyApprovalStorePath(t *testing.T) {
	cfg := Default()
	cfg.Approval.StorePath = ""

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate: expected nil for empty approval.store_path, got %v", err)
	}
}

func TestValidateRejectsApprovalTTLWhenStorePathSet(t *testing.T) {
	cfg := Default()
	cfg.Approval.StorePath = "/tmp/approvals.json"
	cfg.Approval.TTLSeconds = 0

	if err := Validate(cfg); err == nil {
		t.Fatal("Validate: expected error when approval.store_path is set but ttl_seconds is 0")
	}
}

func TestValidateAcceptsApprovalStorePathWithPositiveTTL(t *testing.T) {
	cfg := Default()
	cfg.Approval.StorePath = "/tmp/approvals.json"
	cfg.Approval.TTLSeconds = 900

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate: expected nil, got %v", err)
	}
}

func TestLoadParsesYAML(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte(minimalYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Name != "agentic-velociraptor-mcp" {
		t.Errorf("Server.Name = %q, want %q", cfg.Server.Name, "agentic-velociraptor-mcp")
	}
	if cfg.Policy.Mode != PolicyModeReadOnly {
		t.Errorf("Policy.Mode = %q, want %q", cfg.Policy.Mode, PolicyModeReadOnly)
	}
}

const minimalYAML = `
server:
  name: agentic-velociraptor-mcp
  transport: stdio
velociraptor:
  org_id: root
  read_api_config_path: /tmp/reader.api.config.yaml
  timeout_seconds: 30
  max_rows: 500
  max_result_bytes: 1048576
  max_upload_bytes: 52428800
policy:
  mode: read_only
  allow_raw_vql: false
  max_hunt_clients: 100
audit:
  enabled: false
`
