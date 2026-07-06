package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
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
	if deps.VelociraptorWriteMode != "mock" {
		t.Errorf("VelociraptorWriteMode = %q, want %q", deps.VelociraptorWriteMode, "mock")
	}
	if deps.Approvals != nil {
		t.Error("Approvals != nil, want nil when approval.store_path is empty")
	}
}

func TestBuildDepsConstructsApprovalStoreWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	storePath := filepath.Join(dir, "approvals.json")
	cfgYAML := `
server:
  name: test
  transport: stdio
velociraptor:
  org_id: root
  timeout_seconds: 5
  max_rows: 500
  max_result_bytes: 1048576
  max_upload_bytes: 52428800
policy:
  mode: controlled
  allow_raw_vql: false
  max_hunt_clients: 100
audit:
  enabled: false
approval:
  store_path: ` + storePath + `
  ttl_seconds: 900
`
	if err := os.WriteFile(cfgPath, []byte(cfgYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	deps, _, err := buildDeps(cfgPath, "../../profiles")
	if err != nil {
		t.Fatalf("buildDeps: %v", err)
	}
	if deps.Approvals == nil {
		t.Fatal("Approvals = nil, want non-nil when approval.store_path is configured")
	}
}

func TestRunApproveRequiresStore(t *testing.T) {
	var buf bytes.Buffer
	code := run([]string{"approve", "--reference", "ref-1", "--operation", "collect_artifact"}, &buf)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(buf.String(), "--store is required") {
		t.Fatalf("output %q does not mention --store is required", buf.String())
	}
}

func TestRunApproveRejectsUnknownOperation(t *testing.T) {
	var buf bytes.Buffer
	code := run([]string{"approve", "--store", filepath.Join(t.TempDir(), "approvals.json"), "--reference", "ref-1", "--operation", "bogus"}, &buf)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if !strings.Contains(buf.String(), "--operation must be one of") {
		t.Fatalf("output %q does not mention valid operations", buf.String())
	}
}

func TestRunApproveCreatesAndApprovesRequest(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "approvals.json")

	var buf bytes.Buffer
	code := run([]string{
		"approve",
		"--store", storePath,
		"--reference", "CASE-1234-01",
		"--operation", "collect_artifact",
		"--case-id", "CASE-1234",
		"--reason", "triage",
		"--requester", "analyst@example.com",
		"--client-id", "C.1234abcd5678ef90",
		"--artifact", "Generic.Client.Info",
		"--approved-by", "ir-lead@example.com",
	}, &buf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0: %s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "CASE-1234-01") || !strings.Contains(buf.String(), "approved") {
		t.Fatalf("output %q does not confirm approval", buf.String())
	}

	if _, err := os.Stat(storePath); err != nil {
		t.Fatalf("approval store file not created: %v", err)
	}
}

func TestRunApproveSupportsDeny(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "approvals.json")

	var buf bytes.Buffer
	code := run([]string{
		"approve",
		"--store", storePath,
		"--reference", "CASE-1234-02",
		"--operation", "cancel_flow",
		"--case-id", "CASE-1234",
		"--reason", "stop runaway collection",
		"--requester", "analyst@example.com",
		"--client-id", "C.1234abcd5678ef90",
		"--flow-id", "F.BN2HJC4N4T6KG",
		"--approved-by", "ir-lead@example.com",
		"--deny",
		"--note", "not justified",
	}, &buf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0: %s", code, buf.String())
	}
	if !strings.Contains(buf.String(), "denied") {
		t.Fatalf("output %q does not confirm denial", buf.String())
	}
}

func TestRunApproveCreatesStartHuntRequestWithScope(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "approvals.json")

	var buf bytes.Buffer
	code := run([]string{
		"approve",
		"--store", storePath,
		"--reference", "CASE-9000-01",
		"--operation", "start_hunt",
		"--case-id", "CASE-9000",
		"--reason", "sweep for lateral movement",
		"--requester", "analyst@example.com",
		"--artifact", "Windows.System.Pslist",
		"--hunt-client-id", "C.1111111111111111",
		"--hunt-client-id", "C.2222222222222222",
		"--label", "windows",
		"--approved-by", "ir-lead@example.com",
	}, &buf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0: %s", code, buf.String())
	}

	store, err := approval.NewFileStore(storePath, time.Hour)
	if err != nil {
		t.Fatalf("approval.NewFileStore: %v", err)
	}
	status, err := store.Get(context.Background(), "CASE-9000-01")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if status.Request.Operation != approval.OperationStartHunt {
		t.Errorf("Operation = %q, want %q", status.Request.Operation, approval.OperationStartHunt)
	}
	if status.Request.Artifact != "Windows.System.Pslist" {
		t.Errorf("Artifact = %q, want Windows.System.Pslist", status.Request.Artifact)
	}
	if status.Request.Label != "windows" {
		t.Errorf("Label = %q, want windows", status.Request.Label)
	}
	if len(status.Request.ClientIDs) != 2 {
		t.Fatalf("ClientIDs = %v, want 2 entries", status.Request.ClientIDs)
	}
}

func TestRunApproveCreatesCancelHuntRequest(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "approvals.json")

	var buf bytes.Buffer
	code := run([]string{
		"approve",
		"--store", storePath,
		"--reference", "CASE-9000-02",
		"--operation", "cancel_hunt",
		"--case-id", "CASE-9000",
		"--reason", "stop runaway hunt",
		"--requester", "analyst@example.com",
		"--hunt-id", "H.1234abcd5678ef90",
		"--approved-by", "ir-lead@example.com",
	}, &buf)
	if code != 0 {
		t.Fatalf("exit code = %d, want 0: %s", code, buf.String())
	}

	store, err := approval.NewFileStore(storePath, time.Hour)
	if err != nil {
		t.Fatalf("approval.NewFileStore: %v", err)
	}
	status, err := store.Get(context.Background(), "CASE-9000-02")
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if status.Request.HuntID != "H.1234abcd5678ef90" {
		t.Errorf("HuntID = %q, want H.1234abcd5678ef90", status.Request.HuntID)
	}
}
