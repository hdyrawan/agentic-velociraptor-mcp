package dfir

import "testing"

// fakeAllowlist allows every artifact; used to isolate registry/parsing
// tests from policy allowlist content.
type fakeAllowlist struct{ allow map[string]bool }

func (f fakeAllowlist) ArtifactAllowed(name string) bool { return f.allow[name] }

func TestLoadDirParsesShippedProfiles(t *testing.T) {
	reg, err := LoadDir("../../profiles")
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	want := []string{"windows_basic_triage", "windows_ransomware_triage", "linux_basic_triage"}
	for _, name := range want {
		p, ok := reg.Get(name)
		if !ok {
			t.Errorf("profile %q not loaded", name)
			continue
		}
		if len(p.Artifacts) == 0 {
			t.Errorf("profile %q has no artifacts", name)
		}
	}

	if got := len(reg.List()); got != len(want) {
		t.Errorf("List() returned %d profiles, want %d", got, len(want))
	}
}

func TestValidateProfileRejectsNonAllowlistedArtifact(t *testing.T) {
	p := Profile{
		Name: "test_profile",
		Artifacts: []ProfileArtifact{
			{Name: "Generic.Client.Info"},
			{Name: "Not.Allowed.Artifact"},
		},
	}
	allow := fakeAllowlist{allow: map[string]bool{"Generic.Client.Info": true}}

	if err := ValidateProfile(p, allow); err == nil {
		t.Fatal("ValidateProfile: expected error for non-allowlisted artifact, got nil")
	}
}

func TestValidateProfileAcceptsFullyAllowlistedProfile(t *testing.T) {
	p := Profile{
		Name: "test_profile",
		Artifacts: []ProfileArtifact{
			{Name: "Generic.Client.Info"},
		},
	}
	allow := fakeAllowlist{allow: map[string]bool{"Generic.Client.Info": true}}

	if err := ValidateProfile(p, allow); err != nil {
		t.Fatalf("ValidateProfile: %v", err)
	}
}
