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
func ValidateProfile(p Profile, allow ArtifactAllowlister) error {
	if err := validateProfileMetadata(p); err != nil {
		return err
	}
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
