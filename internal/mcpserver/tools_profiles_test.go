package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
)

var expectedShippedDFIRProfileNames = []string{
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
}

var disallowedDFIRProfileNameSubstrings = []string{
	"CmdShell",
	"execve",
	"Shell",
	"PowershellExec",
	"Command",
}

func TestListDFIRProfilesHandlerReturnsShippedProfiles(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newListDFIRProfilesHandler(deps)

	_, out, err := handler(context.Background(), nil, ListDFIRProfilesInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if got, want := len(out.Profiles), len(expectedShippedDFIRProfileNames); got != want {
		t.Fatalf("got %d profiles, want %d: %+v", got, want, out.Profiles)
	}

	seen := make(map[string]bool, len(out.Profiles))
	for _, profile := range out.Profiles {
		if seen[profile.Name] {
			t.Errorf("duplicate profile name %q", profile.Name)
		}
		seen[profile.Name] = true

		if profile.ArtifactCount == 0 {
			t.Errorf("profile %q has no artifacts", profile.Name)
		}
		if profile.RiskLevel == "" {
			t.Errorf("profile %q has empty risk_level", profile.Name)
		}
		assertNoDisallowedDFIRProfileSubstrings(t, "profile name", profile.Name)
	}

	getHandler := newGetDFIRProfileHandler(deps)
	for _, name := range expectedShippedDFIRProfileNames {
		if !seen[name] {
			t.Errorf("profile %q not returned", name)
		}

		_, profileOut, err := getHandler(context.Background(), nil, GetDFIRProfileInput{Name: name})
		if err != nil {
			t.Fatalf("get profile %q: %v", name, err)
		}
		if len(profileOut.Profile.Artifacts) == 0 {
			t.Errorf("profile %q has no artifacts", name)
		}
		if profileOut.Profile.RiskLevel == "" {
			t.Errorf("profile %q has empty risk_level", name)
		}
		if profileOut.Profile.MaxRuntimeSeconds <= 0 {
			t.Errorf("profile %q has non-positive max_runtime_seconds", name)
		}
		if profileOut.Profile.MaxResultRows <= 0 {
			t.Errorf("profile %q has non-positive max_result_rows", name)
		}
		if profileOut.Profile.MaxResultBytes <= 0 {
			t.Errorf("profile %q has non-positive max_result_bytes", name)
		}
		for _, artifact := range profileOut.Profile.Artifacts {
			assertNoDisallowedDFIRProfileSubstrings(t, "artifact name", artifact.Name)
		}
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event = %+v, ok=%v, want success", evt, ok)
	}
}

func assertNoDisallowedDFIRProfileSubstrings(t *testing.T, field, value string) {
	t.Helper()
	for _, forbidden := range disallowedDFIRProfileNameSubstrings {
		if strings.Contains(value, forbidden) {
			t.Errorf("%s %q contains disallowed substring %q", field, value, forbidden)
		}
	}
}

func TestGetDFIRProfileHandlerFound(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newGetDFIRProfileHandler(deps)

	_, out, err := handler(context.Background(), nil, GetDFIRProfileInput{Name: "windows_basic_triage"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Profile.Name != "windows_basic_triage" {
		t.Errorf("Profile.Name = %q, want %q", out.Profile.Name, "windows_basic_triage")
	}
	if len(out.Profile.Artifacts) == 0 {
		t.Error("Profile.Artifacts is empty")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event = %+v, ok=%v, want success", evt, ok)
	}
}

func TestGetDFIRProfileHandlerNotFoundReturnsSafeError(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newGetDFIRProfileHandler(deps)

	_, _, err := handler(context.Background(), nil, GetDFIRProfileInput{Name: "does_not_exist"})
	if err == nil {
		t.Fatal("handler: expected error for unknown profile, got nil")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError {
		t.Errorf("audit event = %+v, ok=%v, want error outcome", evt, ok)
	}
}

func TestGetDFIRProfileHandlerRejectsInvalidNameSyntax(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newGetDFIRProfileHandler(deps)

	_, _, err := handler(context.Background(), nil, GetDFIRProfileInput{Name: "Not Valid; DROP"})
	if err == nil {
		t.Fatal("handler: expected error for malformed profile name, got nil")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event = %+v, ok=%v, want blocked outcome", evt, ok)
	}
}

func TestValidateDFIRProfileHandlerValid(t *testing.T) {
	deps, _ := testDeps(t)
	handler := newValidateDFIRProfileHandler(deps)

	_, out, err := handler(context.Background(), nil, ValidateDFIRProfileInput{Name: "windows_basic_triage"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !out.Valid {
		t.Errorf("Valid = false, want true (windows_basic_triage only uses default-allowlisted artifacts): %+v", out)
	}
}

func TestValidateDFIRProfileHandlerInvalid(t *testing.T) {
	deps, _ := testDeps(t)
	handler := newValidateDFIRProfileHandler(deps)

	// windows_ransomware_triage references artifacts not present in
	// config.Default()'s allowlist, so it must validate as invalid
	// rather than failing the tool call outright.
	_, out, err := handler(context.Background(), nil, ValidateDFIRProfileInput{Name: "windows_ransomware_triage"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Valid {
		t.Error("Valid = true, want false (profile references non-allowlisted artifacts)")
	}
	if out.Error == "" {
		t.Error("Error is empty, want an explanation")
	}
}

func TestValidateDFIRProfileHandlerNotFound(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newValidateDFIRProfileHandler(deps)

	_, _, err := handler(context.Background(), nil, ValidateDFIRProfileInput{Name: "does_not_exist"})
	if err == nil {
		t.Fatal("handler: expected error for unknown profile, got nil")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError {
		t.Errorf("audit event = %+v, ok=%v, want error outcome", evt, ok)
	}
}
