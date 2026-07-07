package dfir

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// CatalogArtifact is one entry in the curated artifact catalog: a single
// reviewed Velociraptor artifact name plus the metadata a profile author
// (and a reviewer) needs to reason about it. The catalog is an
// authoring-time and test-time registry only — it is NOT a runtime
// permission path. Whether an artifact may actually be collected is still
// gated exclusively by config.PolicyConfig.AllowedArtifacts at runtime
// (see internal/policy and ValidateProfile); the catalog neither widens
// nor bypasses that allowlist. Its purpose is to guarantee that every
// artifact a profile references is a known, reviewed name with recorded
// OS/category/risk/sensitivity/verification metadata, rather than a
// free-form or guessed string.
type CatalogArtifact struct {
	// Name is the exact Velociraptor artifact name, e.g.
	// "Windows.System.Pslist". Never a wildcard, prefix, or pattern.
	Name string `yaml:"name"`

	// TargetOS is "windows", "linux", "macos", or "any".
	TargetOS string `yaml:"target_os"`

	// Category loosely groups artifacts for documentation, e.g.
	// "inventory", "process", "network", "persistence", "eventlog",
	// "browser", "timeline", "ioc".
	Category string `yaml:"category"`

	// RiskLevel classifies operational risk/volume: "low", "medium", or
	// "high".
	RiskLevel string `yaml:"risk_level"`

	// RequiresApproval records whether an artifact is considered
	// sensitive enough that any profile containing it should itself be
	// approval-gated. This is advisory metadata for profile authors; the
	// actual approval enforcement is per-tool (see
	// internal/mcpserver) and per-profile (Profile.RequiresApproval).
	RequiresApproval bool `yaml:"requires_approval"`

	// Sensitivity is a short human label describing the privacy/impact
	// character of the data this artifact returns, e.g. "none", "system",
	// "user-activity", "credential-adjacent".
	Sensitivity string `yaml:"sensitivity"`

	// Verified is true when the artifact name has been confirmed to exist
	// in a real Velociraptor server's artifact catalog (see notes / the
	// docs). When false the entry is a candidate/illustrative name that
	// must not be treated as production-ready without confirmation
	// against the target deployment's catalog.
	Verified bool `yaml:"verified"`

	// Notes is free-form reviewer context: what the artifact does, why a
	// risk/sensitivity/approval choice was made, or why it is unverified.
	Notes string `yaml:"notes"`
}

// Catalog is the loaded set of curated artifacts, keyed by exact name.
type Catalog struct {
	artifacts map[string]CatalogArtifact
}

// catalogFile is the on-disk YAML shape: a single top-level "artifacts"
// list. Kept separate from Catalog so the in-memory form can index by
// name.
type catalogFile struct {
	Artifacts []CatalogArtifact `yaml:"artifacts"`
}

// LoadCatalog reads a curated artifact catalog YAML file. Unknown fields
// are rejected (KnownFields) so a typo in a metadata key fails loudly at
// load time rather than silently dropping metadata. Duplicate artifact
// names are rejected.
func LoadCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("dfir: read artifact catalog %s: %w", path, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cf catalogFile
	if err := dec.Decode(&cf); err != nil {
		return nil, fmt.Errorf("dfir: parse artifact catalog %s: %w", path, err)
	}

	artifacts := make(map[string]CatalogArtifact, len(cf.Artifacts))
	for _, a := range cf.Artifacts {
		if err := validateCatalogArtifact(a); err != nil {
			return nil, fmt.Errorf("dfir: artifact catalog %s: %w", path, err)
		}
		if _, dup := artifacts[a.Name]; dup {
			return nil, fmt.Errorf("dfir: artifact catalog %s: duplicate artifact %q", path, a.Name)
		}
		artifacts[a.Name] = a
	}
	if len(artifacts) == 0 {
		return nil, fmt.Errorf("dfir: artifact catalog %s: no artifacts defined", path)
	}

	return &Catalog{artifacts: artifacts}, nil
}

func validateCatalogArtifact(a CatalogArtifact) error {
	if a.Name == "" {
		return fmt.Errorf("catalog artifact name is required")
	}
	switch a.TargetOS {
	case "windows", "linux", "macos", "any":
	default:
		return fmt.Errorf("catalog artifact %q target_os must be one of windows, linux, macos, any", a.Name)
	}
	switch a.RiskLevel {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("catalog artifact %q risk_level must be one of low, medium, high", a.Name)
	}
	if a.Category == "" {
		return fmt.Errorf("catalog artifact %q category is required", a.Name)
	}
	if a.Sensitivity == "" {
		return fmt.Errorf("catalog artifact %q sensitivity is required", a.Name)
	}
	return nil
}

// Has reports whether name is a curated artifact.
func (c *Catalog) Has(name string) bool {
	_, ok := c.artifacts[name]
	return ok
}

// Get returns the catalog entry for name, if present.
func (c *Catalog) Get(name string) (CatalogArtifact, bool) {
	a, ok := c.artifacts[name]
	return a, ok
}

// Len returns the number of curated artifacts.
func (c *Catalog) Len() int { return len(c.artifacts) }

// Names returns all curated artifact names sorted.
func (c *Catalog) Names() []string {
	names := make([]string, 0, len(c.artifacts))
	for n := range c.artifacts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ValidateProfileAgainstCatalog checks that every artifact referenced by
// p exists in the curated catalog. This is a defense-in-depth,
// authoring-time invariant layered on top of (never replacing) the
// runtime policy.allowed_artifacts allowlist enforced by ValidateProfile:
// a profile may only reference names that a human reviewer has already
// recorded in the catalog with OS/risk/sensitivity/verification metadata,
// closing the door on a profile silently introducing an unreviewed or
// mistyped artifact name.
func ValidateProfileAgainstCatalog(p Profile, c *Catalog) error {
	if len(p.Artifacts) == 0 {
		return fmt.Errorf("dfir: profile %q defines no artifacts", p.Name)
	}
	for _, a := range p.Artifacts {
		if !c.Has(a.Name) {
			return fmt.Errorf("dfir: profile %q references artifact %q not present in the curated artifact catalog", p.Name, a.Name)
		}
	}
	return nil
}
