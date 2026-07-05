package dfir

import "fmt"

// ArtifactAllowlister is satisfied by policy.Engine; declared narrowly
// here to avoid an import cycle between internal/dfir and
// internal/policy.
type ArtifactAllowlister interface {
	ArtifactAllowed(name string) bool
}

// ValidateProfile checks that every artifact referenced by p is present
// in the given allowlist. A DFIR profile must never grant access to an
// artifact that individual artifact-level policy would refuse; profiles
// are a convenience grouping, not an escalation path.
//
// TODO(v0.4.0): extend with schema checks (known TargetOS values,
// non-empty Artifacts, parameter key sanity) once
// velo_validate_dfir_profile is implemented.
func ValidateProfile(p Profile, allow ArtifactAllowlister) error {
	if len(p.Artifacts) == 0 {
		return fmt.Errorf("dfir: profile %q defines no artifacts", p.Name)
	}
	for _, a := range p.Artifacts {
		if !allow.ArtifactAllowed(a.Name) {
			return fmt.Errorf("dfir: profile %q references non-allowlisted artifact %q", p.Name, a.Name)
		}
	}
	return nil
}
