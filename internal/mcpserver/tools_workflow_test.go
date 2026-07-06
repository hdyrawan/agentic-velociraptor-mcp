package mcpserver

import (
	"context"
	"testing"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/response"
)

func TestPlanDFIRTriageHandlerSuccess(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newPlanDFIRTriageHandler(deps)

	_, out, err := handler(context.Background(), nil, PlanDFIRTriageInput{
		CaseType: "ransomware",
		TargetOS: "windows",
		ClientID: "C.1234abcd5678ef90",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Fatalf("Status = %q, want success: %+v", out.Status, out)
	}
	if out.ClientID != "C.1234abcd5678ef90" {
		t.Errorf("ClientID = %q", out.ClientID)
	}
	if len(out.Recommendations) == 0 {
		t.Fatal("Recommendations is empty")
	}
	found := false
	for _, rec := range out.Recommendations {
		if rec.Name == "windows_ransomware_triage" {
			found = true
			if rec.ArtifactCount == 0 {
				t.Errorf("windows_ransomware_triage ArtifactCount = 0")
			}
			if len(rec.Reasons) == 0 {
				t.Errorf("windows_ransomware_triage Reasons is empty")
			}
		}
	}
	if !found {
		t.Fatalf("windows_ransomware_triage not recommended: %+v", out.Recommendations)
	}
	if len(out.ReadOnlyNextSteps) == 0 || len(out.SafetyNotes) == 0 {
		t.Fatalf("read-only guidance missing: %+v", out)
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess || evt.Tool != "velo_plan_dfir_triage" {
		t.Errorf("audit event = %+v, ok=%v, want success plan event", evt, ok)
	}
}

func TestPlanDFIRTriageHandlerEmpty(t *testing.T) {
	deps, _ := testDeps(t)
	handler := newPlanDFIRTriageHandler(deps)

	_, out, err := handler(context.Background(), nil, PlanDFIRTriageInput{CaseType: "eventlog", TargetOS: "linux"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusEmpty {
		t.Fatalf("Status = %q, want empty: %+v", out.Status, out)
	}
	if len(out.Recommendations) != 0 {
		t.Fatalf("Recommendations = %+v, want empty", out.Recommendations)
	}
}

func TestPlanDFIRTriageHandlerRejectsInvalidClientID(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newPlanDFIRTriageHandler(deps)

	_, _, err := handler(context.Background(), nil, PlanDFIRTriageInput{ClientID: "not-valid"})
	if err == nil {
		t.Fatal("handler: expected invalid client error, got nil")
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event = %+v, ok=%v, want blocked", evt, ok)
	}
}

func TestCompareDFIRProfilesHandlerSuccess(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newCompareDFIRProfilesHandler(deps)

	_, out, err := handler(context.Background(), nil, CompareDFIRProfilesInput{Names: []string{"windows_basic_triage", "windows_process_network_triage"}})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Fatalf("Status = %q, want success: %+v", out.Status, out)
	}
	if len(out.Profiles) != 2 {
		t.Fatalf("Profiles len = %d, want 2", len(out.Profiles))
	}
	if out.UniqueArtifacts == nil {
		t.Fatal("UniqueArtifacts is nil")
	}
	for _, profile := range out.Profiles {
		if len(profile.Artifacts) == 0 {
			t.Errorf("profile %q has no artifacts", profile.Name)
		}
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess || evt.Tool != "velo_compare_dfir_profiles" {
		t.Errorf("audit event = %+v, ok=%v, want success compare event", evt, ok)
	}
}

func TestCompareDFIRProfilesHandlerNotFound(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newCompareDFIRProfilesHandler(deps)

	_, out, err := handler(context.Background(), nil, CompareDFIRProfilesInput{Names: []string{"windows_basic_triage", "does_not_exist"}})
	if err != nil {
		t.Fatalf("handler returned Go error, want structured not_found: %v", err)
	}
	if out.Status != response.StatusNotFound {
		t.Fatalf("Status = %q, want not_found: %+v", out.Status, out)
	}
	if len(out.MissingProfiles) != 1 || out.MissingProfiles[0] != "does_not_exist" {
		t.Fatalf("MissingProfiles = %+v", out.MissingProfiles)
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeError {
		t.Errorf("audit event = %+v, ok=%v, want error outcome for not_found", evt, ok)
	}
}

func TestCompareDFIRProfilesHandlerRejectsInvalidInput(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newCompareDFIRProfilesHandler(deps)

	_, _, err := handler(context.Background(), nil, CompareDFIRProfilesInput{Names: []string{"windows_basic_triage"}})
	if err == nil {
		t.Fatal("handler: expected too-few-profiles error, got nil")
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event = %+v, ok=%v, want blocked", evt, ok)
	}
}

func TestFindProfilesByArtifactHandlerSuccess(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newFindProfilesByArtifactHandler(deps)

	_, out, err := handler(context.Background(), nil, FindProfilesByArtifactInput{Artifact: "Generic.Client.Info"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Status != response.StatusSuccess {
		t.Fatalf("Status = %q, want success: %+v", out.Status, out)
	}
	if !out.ArtifactAllowed {
		t.Error("ArtifactAllowed = false, want true for Default policy artifact")
	}
	if len(out.Profiles) == 0 {
		t.Fatal("Profiles is empty")
	}

	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeSuccess || evt.Tool != "velo_find_profiles_by_artifact" {
		t.Errorf("audit event = %+v, ok=%v, want success artifact event", evt, ok)
	}
}

func TestFindProfilesByArtifactHandlerNotFound(t *testing.T) {
	deps, _ := testDeps(t)
	handler := newFindProfilesByArtifactHandler(deps)

	_, out, err := handler(context.Background(), nil, FindProfilesByArtifactInput{Artifact: "Custom.Does.Not.Exist"})
	if err != nil {
		t.Fatalf("handler returned Go error, want structured not_found: %v", err)
	}
	if out.Status != response.StatusNotFound {
		t.Fatalf("Status = %q, want not_found: %+v", out.Status, out)
	}
	if len(out.Profiles) != 0 {
		t.Fatalf("Profiles = %+v, want empty", out.Profiles)
	}
}

func TestFindProfilesByArtifactHandlerRejectsInvalidArtifact(t *testing.T) {
	deps, sink := testDeps(t)
	handler := newFindProfilesByArtifactHandler(deps)

	_, _, err := handler(context.Background(), nil, FindProfilesByArtifactInput{Artifact: "not valid; drop"})
	if err == nil {
		t.Fatal("handler: expected invalid artifact error, got nil")
	}
	evt, ok := sink.last()
	if !ok || evt.Outcome != audit.OutcomeBlocked {
		t.Errorf("audit event = %+v, ok=%v, want blocked", evt, ok)
	}
}
