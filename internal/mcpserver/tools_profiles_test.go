package mcpserver

import (
	"context"
	"testing"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
)

func TestListDFIRProfilesHandlerReturnsShippedProfiles(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newListDFIRProfilesHandler(deps)

	_, out, err := handler(context.Background(), nil, ListDFIRProfilesInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}

	if len(out.Profiles) != 3 {
		t.Fatalf("got %d profiles, want 3: %+v", len(out.Profiles), out.Profiles)
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess {
		t.Errorf("audit event = %+v, ok=%v, want success", evt, ok)
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
