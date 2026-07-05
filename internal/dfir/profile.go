// Package dfir defines DFIR investigation profiles: named, reviewed
// bundles of allowlisted Velociraptor artifacts that correspond to a
// specific investigation intent (triage, ransomware, credential theft,
// IOC hunting, ...).
//
// Profiles exist so an agent (and the human approving its requests)
// reasons about "run the ransomware triage profile against C.abc123",
// not about individual artifact names or, worse, raw VQL. A profile is
// itself just a curated list of already-allowlisted artifacts plus
// metadata; it grants no permission beyond what the artifact allowlist
// and Velociraptor ACLs already allow.
package dfir

// Profile is a named, versioned bundle of artifacts appropriate for a
// specific DFIR use case.
type Profile struct {
	// Name is the stable identifier, e.g. "windows_basic_triage". Must
	// match the YAML filename (without extension) under profiles/.
	Name string `yaml:"name"`

	// DisplayName / Description are human-facing.
	DisplayName string `yaml:"display_name"`
	Description string `yaml:"description"`

	// TargetOS constrains which endpoints this profile is meaningful
	// for: "windows", "linux", or "any" (for cross-platform profiles
	// like IOC hunts).
	TargetOS string `yaml:"target_os"`

	// Artifacts lists the Velociraptor artifact names this profile
	// collects, in collection order. Every entry must also appear in
	// config.PolicyConfig.AllowedArtifacts at runtime, or the profile is
	// rejected; see validate.go.
	Artifacts []ProfileArtifact `yaml:"artifacts"`

	// Category loosely groups profiles for listing/documentation, e.g.
	// "triage", "ransomware", "ioc", "timeline".
	Category string `yaml:"category"`
}

// ProfileArtifact is one artifact entry within a profile, with any fixed
// (non-agent-controlled) parameters the profile wants to pass.
type ProfileArtifact struct {
	Name string `yaml:"name"`

	// Parameters are fixed key/value pairs baked into the profile
	// definition itself (reviewed at profile authoring time), not
	// supplied by the calling agent. Agent-supplied values are a
	// separate, narrower path; see internal/vql/env.go.
	Parameters map[string]string `yaml:"parameters,omitempty"`
}
