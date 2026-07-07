package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/config"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/validation"
)

// exampleServerConfigs returns every shipped example *server* config
// (examples/config/*.yaml plus examples/client-configs/config.*.yaml).
// reader.api.config.example.yaml is deliberately excluded: it is an
// example of Velociraptor's api.config.yaml shape, not this project's
// config schema.
func exampleServerConfigs(t *testing.T) []string {
	t.Helper()

	var paths []string
	for _, pattern := range []string{
		filepath.Join("..", "..", "examples", "config", "*.yaml"),
		filepath.Join("..", "..", "examples", "client-configs", "config.*.yaml"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		paths = append(paths, matches...)
	}
	if len(paths) < 3 {
		t.Fatalf("found only %d example server configs (%v), want at least the readonly/controlled/client-config trio", len(paths), paths)
	}
	return paths
}

// requiredApprovalCategories is the full stable set every shipped
// example must keep listed — removing one from an example would
// document a weaker-than-shipped posture.
var requiredApprovalCategories = []string{
	"collect_artifact",
	"collect_dfir_profile",
	"start_hunt",
	"start_dfir_hunt",
	"cancel_flow",
	"cancel_hunt",
	"download_flow_upload",
	"hunt_ioc",
}

// TestExampleConfigsLoadValidateAndStaySafe pins the v0.10.4
// production-readiness guarantee that every shipped example config (a)
// actually loads and passes config.Validate, and (b) cannot quietly
// drift into an unsafe posture: raw VQL off, target-all off, stdio-only
// transport, audit on, exact-name (no wildcard) allowlists, and the
// complete approval-category set.
func TestExampleConfigsLoadValidateAndStaySafe(t *testing.T) {
	for _, path := range exampleServerConfigs(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if err := config.Validate(cfg); err != nil {
				t.Fatalf("Validate: %v", err)
			}

			if cfg.Server.Transport != "stdio" {
				t.Errorf("server.transport = %q, want stdio", cfg.Server.Transport)
			}
			if cfg.Policy.AllowRawVQL {
				t.Error("policy.allow_raw_vql = true; examples must never enable raw VQL")
			}
			if cfg.Policy.AllowTargetAll {
				t.Error("policy.allow_target_all = true; examples must never enable all-clients targeting")
			}
			if cfg.Policy.AllowListAllArtifacts {
				t.Error("policy.allow_list_all_artifacts = true; examples must stay allowlist-scoped")
			}
			if !cfg.Audit.Enabled {
				t.Error("audit.enabled = false; examples must model auditing on")
			}
			if cfg.Policy.MaxHuntClients <= 0 || cfg.Policy.MaxHuntClients > 100 {
				t.Errorf("policy.max_hunt_clients = %d, want in (0, 100] for a shipped example", cfg.Policy.MaxHuntClients)
			}

			// A read-only-named example must actually model the
			// read-only posture, with no write identity configured.
			name := filepath.Base(path)
			if strings.Contains(name, "readonly") || strings.Contains(name, "read-only") {
				if cfg.Policy.Mode != config.PolicyModeReadOnly {
					t.Errorf("policy.mode = %q, want read_only for %s", cfg.Policy.Mode, name)
				}
				if cfg.Velociraptor.WriteAPIConfigPath != "" {
					t.Errorf("velociraptor.write_api_config_path = %q, want empty in a read-only example", cfg.Velociraptor.WriteAPIConfigPath)
				}
				if cfg.Approval.StorePath != "" {
					t.Errorf("approval.store_path = %q, want empty in a read-only example", cfg.Approval.StorePath)
				}
			}

			// Allowlists are exact names only — no wildcard/prefix
			// grammar exists, and no example may pretend one does.
			for _, a := range cfg.Policy.AllowedArtifacts {
				if err := validation.ArtifactName(a); err != nil {
					t.Errorf("allowed_artifacts entry %q is not a valid exact artifact name: %v", a, err)
				}
			}
			for _, p := range cfg.Policy.AllowedProfiles {
				if err := validation.DFIRProfileName(p); err != nil {
					t.Errorf("allowed_profiles entry %q is not a valid exact profile name: %v", p, err)
				}
			}

			got := make(map[string]bool, len(cfg.Policy.RequireApprovalFor))
			for _, c := range cfg.Policy.RequireApprovalFor {
				got[c] = true
			}
			for _, want := range requiredApprovalCategories {
				if !got[want] {
					t.Errorf("policy.require_approval_for is missing %q", want)
				}
			}
		})
	}
}

// TestExampleConfigsCarryNoSecretMaterial guards against an example
// ever being edited to embed real key material or credentials instead
// of a placeholder path.
func TestExampleConfigsCarryNoSecretMaterial(t *testing.T) {
	for _, path := range exampleServerConfigs(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			content := string(data)
			for _, marker := range []string{"BEGIN CERTIFICATE", "PRIVATE KEY"} {
				if strings.Contains(content, marker) {
					t.Errorf("example contains %q; examples must reference secret files by placeholder path, never embed material", marker)
				}
			}
		})
	}
}
