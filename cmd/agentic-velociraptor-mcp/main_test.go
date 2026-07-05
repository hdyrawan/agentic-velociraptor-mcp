package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var buf bytes.Buffer
	code := run([]string{"--version"}, &buf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(buf.String(), version) {
		t.Fatalf("output %q does not contain version %q", buf.String(), version)
	}
}

func TestRunNoConfigShowsUsage(t *testing.T) {
	var buf bytes.Buffer
	code := run(nil, &buf)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(buf.String(), "Usage:") {
		t.Fatalf("output %q does not contain usage text", buf.String())
	}
}

func TestRunHelp(t *testing.T) {
	var buf bytes.Buffer
	code := run([]string{"--help"}, &buf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(buf.String(), "Usage:") {
		t.Fatalf("output %q does not contain usage text", buf.String())
	}
}

func TestRunMissingConfigFileFailsClosedWithoutStartingServer(t *testing.T) {
	var buf bytes.Buffer
	code := run([]string{"--config", "/nonexistent/config.yaml"}, &buf)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if !strings.Contains(buf.String(), "agentic-velociraptor-mcp:") {
		t.Fatalf("output %q does not contain expected error prefix", buf.String())
	}
}

func TestRunInvalidReadAPIConfigFailsClosedWithoutStartingServer(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfgYAML := `
server:
  name: test
  transport: stdio
velociraptor:
  org_id: root
  read_api_config_path: ` + filepath.Join(dir, "does-not-exist.yaml") + `
  timeout_seconds: 5
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
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	code := run([]string{"--config", cfgPath, "--profiles-dir", "../../profiles"}, &buf)
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (fail closed on a configured-but-broken read_api_config_path)", code)
	}
}

func TestResolveProfilesDirExplicitIsNeverOverridden(t *testing.T) {
	got := resolveProfilesDir("/does/not/exist", true)
	if got != "/does/not/exist" {
		t.Errorf("resolveProfilesDir(explicit) = %q, want unchanged", got)
	}
}

func TestResolveProfilesDirPrefersCWDRelative(t *testing.T) {
	got := resolveProfilesDir("../../profiles", false)
	if got != "../../profiles" {
		t.Errorf("resolveProfilesDir = %q, want the cwd-relative path since it exists", got)
	}
}

func TestResolveProfilesDirFallsBackToExecutableDir(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	marker := "resolve-profiles-dir-test-marker"
	markerPath := filepath.Join(filepath.Dir(exe), marker)
	if err := os.Mkdir(markerPath, 0o700); err != nil {
		t.Skipf("cannot write next to test executable: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(markerPath) })

	got := resolveProfilesDir(marker, false)
	if got != markerPath {
		t.Errorf("resolveProfilesDir = %q, want %q (executable-relative fallback)", got, markerPath)
	}
}

func TestResolveProfilesDirFallsThroughWhenNothingResolves(t *testing.T) {
	got := resolveProfilesDir("definitely-does-not-exist-anywhere", false)
	if got != "definitely-does-not-exist-anywhere" {
		t.Errorf("resolveProfilesDir = %q, want the original value unchanged", got)
	}
}

func TestBuildDepsMockModeWhenReadAPIConfigPathEmpty(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfgYAML := `
server:
  name: test
  transport: stdio
velociraptor:
  org_id: root
  read_api_config_path: ""
  timeout_seconds: 5
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
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	deps, _, err := buildDeps(cfgPath, "../../profiles")
	if err != nil {
		t.Fatalf("buildDeps: %v", err)
	}
	if deps.VelociraptorReadMode != "mock" {
		t.Errorf("VelociraptorReadMode = %q, want %q", deps.VelociraptorReadMode, "mock")
	}
}
