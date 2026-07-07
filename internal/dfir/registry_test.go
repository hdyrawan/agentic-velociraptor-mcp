package dfir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeAllowlist allows every artifact; used to isolate registry/parsing
// tests from policy allowlist content.
type fakeAllowlist struct{ allow map[string]bool }

func (f fakeAllowlist) ArtifactAllowed(name string) bool { return f.allow[name] }

var expectedShippedProfileNames = []string{
	// v0.4.0-v0.7.0 original catalog (15).
	"windows_basic_triage",
	"windows_process_network_triage",
	"windows_persistence_triage",
	"windows_lateral_movement_triage",
	"windows_ransomware_triage",
	"windows_credential_theft_triage",
	"windows_eventlog_triage",
	"windows_browser_activity_triage",
	"windows_timeline_triage",
	"linux_basic_triage",
	"linux_process_network_triage",
	"linux_persistence_triage",
	"ioc_hash_hunt",
	"ioc_ip_hunt",
	"ioc_domain_hunt",
	// v0.10.0 curated, catalog-verified expansion (31).
	// Windows.
	"windows_system_inventory",
	"windows_powershell_activity",
	"windows_scheduled_task_persistence",
	"windows_wmi_persistence",
	"windows_service_persistence",
	"windows_execution_evidence",
	"windows_authentication_events",
	"windows_user_activity",
	"windows_network_connections",
	"windows_filesystem_timeline",
	// Windows browser sub-profiles.
	"windows_browser_history",
	"windows_browser_downloads",
	"windows_browser_extensions",
	"windows_browser_cookies",
	"windows_browser_cache",
	// Linux.
	"linux_system_inventory",
	"linux_process_analysis",
	"linux_network_connections",
	"linux_auth_logs",
	"linux_ssh_trust",
	"linux_privilege_escalation",
	"linux_shell_history",
	"linux_cron_persistence",
	"linux_systemd_services",
	"linux_package_inventory",
	"linux_container_triage",
	// Cross-platform.
	"cross_platform_identity",
	"cross_platform_process",
	"cross_platform_network",
	"cross_platform_ioc_context",
	"cross_platform_local_hashes",
}

var disallowedProfileNameSubstrings = []string{
	"CmdShell",
	"execve",
	"Shell",
	"PowershellExec",
	"Command",
}

func TestLoadDirParsesShippedProfiles(t *testing.T) {
	reg, err := LoadDir("../../profiles")
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	profiles := reg.List()
	if got, want := len(profiles), len(expectedShippedProfileNames); got != want {
		t.Fatalf("List() returned %d profiles, want %d", got, want)
	}

	seen := make(map[string]bool, len(profiles))
	for _, p := range profiles {
		if seen[p.Name] {
			t.Errorf("duplicate profile name %q", p.Name)
		}
		seen[p.Name] = true

		if len(p.Artifacts) == 0 {
			t.Errorf("profile %q has no artifacts", p.Name)
		}
		if p.RiskLevel == "" {
			t.Errorf("profile %q has empty risk_level", p.Name)
		}
		if p.MaxRuntimeSeconds <= 0 {
			t.Errorf("profile %q has non-positive max_runtime_seconds", p.Name)
		}
		if p.MaxResultRows <= 0 {
			t.Errorf("profile %q has non-positive max_result_rows", p.Name)
		}
		if p.MaxResultBytes <= 0 {
			t.Errorf("profile %q has non-positive max_result_bytes", p.Name)
		}

		assertNoDisallowedProfileSubstrings(t, "profile name", p.Name)
		for _, artifact := range p.Artifacts {
			assertNoDisallowedProfileSubstrings(t, "artifact name", artifact.Name)
		}
	}

	for _, name := range expectedShippedProfileNames {
		p, ok := reg.Get(name)
		if !ok {
			t.Errorf("profile %q not loaded", name)
			continue
		}
		if len(p.Artifacts) == 0 {
			t.Errorf("profile %q has no artifacts", name)
		}
	}
}

func TestLoadDirRejectsMissingRequiredMetadata(t *testing.T) {
	dir := t.TempDir()
	writeProfileFile(t, dir, "missing_approval.yaml", `
name: missing_approval
display_name: Missing Approval
description: Missing explicit approval metadata.
target_os: windows
category: triage
risk_level: low
max_runtime_seconds: 300
max_result_rows: 1000
max_result_bytes: 5242880
artifacts:
  - name: Generic.Client.Info
`)

	if _, err := LoadDir(dir); err == nil || !strings.Contains(err.Error(), "requires_approval is required") {
		t.Fatalf("LoadDir error = %v, want requires_approval is required", err)
	}
}

func TestLoadDirRejectsUnknownProfileFields(t *testing.T) {
	dir := t.TempDir()
	writeProfileFile(t, dir, "unknown_field.yaml", `
name: unknown_field
display_name: Unknown Field
description: Has an unknown metadata key.
target_os: windows
category: triage
risk_level: low
requires_approval: false
max_runtime_sum: 300
max_runtime_seconds: 300
max_result_rows: 1000
max_result_bytes: 5242880
artifacts:
  - name: Generic.Client.Info
`)

	if _, err := LoadDir(dir); err == nil || !strings.Contains(err.Error(), "field max_runtime_sum not found") {
		t.Fatalf("LoadDir error = %v, want unknown max_runtime_sum field", err)
	}
}

func TestValidateProfileRejectsInvalidMetadata(t *testing.T) {
	p := validTestProfile()
	p.RiskLevel = "critical"
	allow := fakeAllowlist{allow: map[string]bool{"Generic.Client.Info": true}}

	if err := ValidateProfile(p, allow); err == nil || !strings.Contains(err.Error(), "risk_level") {
		t.Fatalf("ValidateProfile error = %v, want risk_level error", err)
	}
}

func assertNoDisallowedProfileSubstrings(t *testing.T, field, value string) {
	t.Helper()
	for _, forbidden := range disallowedProfileNameSubstrings {
		if strings.Contains(value, forbidden) {
			t.Errorf("%s %q contains disallowed substring %q", field, value, forbidden)
		}
	}
}

func TestValidateProfileRejectsNonAllowlistedArtifact(t *testing.T) {
	p := validTestProfile()
	p.Artifacts = []ProfileArtifact{
		{Name: "Generic.Client.Info"},
		{Name: "Not.Allowed.Artifact"},
	}
	allow := fakeAllowlist{allow: map[string]bool{"Generic.Client.Info": true}}

	if err := ValidateProfile(p, allow); err == nil {
		t.Fatal("ValidateProfile: expected error for non-allowlisted artifact, got nil")
	}
}

func TestValidateProfileAcceptsFullyAllowlistedProfile(t *testing.T) {
	p := validTestProfile()
	allow := fakeAllowlist{allow: map[string]bool{"Generic.Client.Info": true}}

	if err := ValidateProfile(p, allow); err != nil {
		t.Fatalf("ValidateProfile: %v", err)
	}
}

func validTestProfile() Profile {
	return Profile{
		Name:              "test_profile",
		Description:       "Test profile.",
		TargetOS:          "windows",
		Category:          "triage",
		RiskLevel:         "low",
		RequiresApproval:  false,
		MaxRuntimeSeconds: 300,
		MaxResultRows:     1000,
		MaxResultBytes:    5242880,
		Artifacts: []ProfileArtifact{
			{Name: "Generic.Client.Info"},
		},
	}
}

func writeProfileFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o600); err != nil {
		t.Fatalf("write profile fixture: %v", err)
	}
}
