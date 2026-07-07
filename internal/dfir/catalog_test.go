package dfir

import (
	"strings"
	"testing"
)

const shippedCatalogPath = "../../catalog/artifacts.yaml"

// TestLoadCatalogParsesShippedCatalog confirms the shipped curated
// artifact catalog loads, has entries, and that every entry carries the
// required reviewed metadata.
func TestLoadCatalogParsesShippedCatalog(t *testing.T) {
	cat, err := LoadCatalog(shippedCatalogPath)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	if cat.Len() == 0 {
		t.Fatal("catalog is empty")
	}
	for _, name := range cat.Names() {
		a, ok := cat.Get(name)
		if !ok {
			t.Fatalf("Names() returned %q but Get() missed it", name)
		}
		// Metadata completeness is enforced by validateCatalogArtifact at
		// load time; re-assert the fields a reviewer most cares about.
		if a.Category == "" || a.Sensitivity == "" {
			t.Errorf("catalog artifact %q missing category/sensitivity", name)
		}
		// No wildcard/prefix/pattern artifact names are ever permitted.
		if strings.ContainsAny(a.Name, "*?") || strings.HasSuffix(a.Name, ".") {
			t.Errorf("catalog artifact %q looks like a wildcard/prefix, not an exact name", a.Name)
		}
	}
}

// TestEveryShippedProfileArtifactIsInCatalog is the core closed-world
// invariant: no profiles/*.yaml file may reference an artifact name that
// is not a reviewed entry in catalog/artifacts.yaml. This runs against
// the real shipped profiles and the real shipped catalog.
func TestEveryShippedProfileArtifactIsInCatalog(t *testing.T) {
	cat, err := LoadCatalog(shippedCatalogPath)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	reg, err := LoadDir("../../profiles")
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	for _, p := range reg.List() {
		if err := ValidateProfileAgainstCatalog(p, cat); err != nil {
			t.Errorf("profile %q: %v", p.Name, err)
		}
	}
}

// TestProfileApprovalConsistentWithCatalog enforces the safety rule that
// any profile referencing an artifact the catalog flags
// requires_approval=true must itself set requires_approval=true. A
// low-friction profile must never bundle in a sensitive artifact and
// thereby collect it without the approval gate the catalog demands.
func TestProfileApprovalConsistentWithCatalog(t *testing.T) {
	cat, err := LoadCatalog(shippedCatalogPath)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	reg, err := LoadDir("../../profiles")
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	for _, p := range reg.List() {
		for _, a := range p.Artifacts {
			ca, ok := cat.Get(a.Name)
			if !ok {
				continue // covered by TestEveryShippedProfileArtifactIsInCatalog
			}
			if ca.RequiresApproval && !p.RequiresApproval {
				t.Errorf("profile %q includes approval-required artifact %q but the profile itself has requires_approval=false", p.Name, a.Name)
			}
		}
	}
}

// TestValidateProfileAgainstCatalogRejectsUnknownArtifact confirms the
// validator fails closed on an artifact absent from the catalog.
func TestValidateProfileAgainstCatalogRejectsUnknownArtifact(t *testing.T) {
	cat, err := LoadCatalog(shippedCatalogPath)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	p := validTestProfile()
	p.Artifacts = []ProfileArtifact{{Name: "Not.A.Catalogued.Artifact"}}
	if err := ValidateProfileAgainstCatalog(p, cat); err == nil {
		t.Fatal("expected error for artifact absent from catalog, got nil")
	}
}

// TestLoadCatalogRejectsDuplicate and unknown-field cases guard the
// loader's own strictness.
func TestLoadCatalogRejectsUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/bad.yaml"
	writeProfileFile(t, dir, "bad.yaml", `
artifacts:
  - name: Generic.Client.Info
    target_os: any
    category: inventory
    risk_level: low
    requires_approval: false
    sensitivity: system
    verified: true
    bogus_field: nope
`)
	if _, err := LoadCatalog(path); err == nil || !strings.Contains(err.Error(), "bogus_field") {
		t.Fatalf("LoadCatalog error = %v, want unknown bogus_field", err)
	}
}
