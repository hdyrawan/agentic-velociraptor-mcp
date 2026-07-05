package validation

import (
	"fmt"
	"regexp"
)

// artifactNamePattern matches Velociraptor artifact names, which are
// dotted, PascalCase-segment identifiers such as "Windows.System.Pslist"
// or "Generic.Client.Info". This intentionally rejects anything that
// could be a VQL fragment (parentheses, whitespace, quotes, operators).
var artifactNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*(\.[A-Za-z][A-Za-z0-9]*)+$`)

// ArtifactName validates the syntactic shape of a Velociraptor artifact
// name. It does not check the name against any allowlist; that is
// policy.Engine.ArtifactAllowed's job. This function exists to reject
// obviously malformed or injection-shaped input before it is even
// compared against the allowlist.
func ArtifactName(name string) error {
	if !artifactNamePattern.MatchString(name) {
		return fmt.Errorf("validation: invalid artifact name %q", name)
	}
	return nil
}

// DFIRProfileNamePattern matches DFIR profile identifiers such as
// "windows_basic_triage": lowercase, digits, and underscores.
var dfirProfileNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// DFIRProfileName validates the syntactic shape of a DFIR profile name.
func DFIRProfileName(name string) error {
	if !dfirProfileNamePattern.MatchString(name) {
		return fmt.Errorf("validation: invalid dfir profile name %q", name)
	}
	return nil
}
