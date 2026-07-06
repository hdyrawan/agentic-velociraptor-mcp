package dfir

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Registry holds the set of loaded, validated DFIR profiles available to
// tool handlers.
//
// TODO(v0.4.0): this is currently a simple in-memory map loaded once at
// startup from the profiles/ directory. Revisit if hot-reload of
// profiles is ever required; the initial design intentionally does not
// support runtime profile mutation, since profile definitions are
// meant to be reviewed, versioned artifacts, not agent-writable state.
type Registry struct {
	profiles map[string]Profile
}

// LoadDir reads every *.yaml file in dir as a Profile and returns a
// Registry. It does not cross-check artifacts against a policy
// allowlist; call Validate (see validate.go) with the relevant
// config.PolicyConfig for that.
func LoadDir(dir string) (*Registry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("dfir: read profiles dir %s: %w", dir, err)
	}

	profiles := make(map[string]Profile)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("dfir: read %s: %w", path, err)
		}
		if err := validateProfileYAMLKeys(data); err != nil {
			return nil, fmt.Errorf("dfir: parse %s: %w", path, err)
		}
		var p Profile
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("dfir: parse %s: %w", path, err)
		}
		if err := validateProfileMetadata(p); err != nil {
			return nil, fmt.Errorf("dfir: %s: %w", path, err)
		}
		if _, dup := profiles[p.Name]; dup {
			return nil, fmt.Errorf("dfir: duplicate profile name %q", p.Name)
		}
		profiles[p.Name] = p
	}

	return &Registry{profiles: profiles}, nil
}

func validateProfileYAMLKeys(data []byte) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("profile YAML must be a mapping")
	}

	seen := make(map[string]bool)
	for i := 0; i < len(doc.Content[0].Content); i += 2 {
		seen[doc.Content[0].Content[i].Value] = true
	}

	for _, required := range []string{
		"name",
		"description",
		"target_os",
		"artifacts",
		"risk_level",
		"requires_approval",
		"max_runtime_seconds",
		"max_result_rows",
		"max_result_bytes",
	} {
		if !seen[required] {
			return fmt.Errorf("%s is required", required)
		}
	}
	return nil
}

func validateProfileMetadata(p Profile) error {
	if p.Name == "" {
		return fmt.Errorf("profile name is required")
	}
	switch p.RiskLevel {
	case "low", "medium", "high":
	default:
		return fmt.Errorf("profile %q risk_level must be one of low, medium, high", p.Name)
	}
	if p.MaxRuntimeSeconds <= 0 || p.MaxRuntimeSeconds > 86400 {
		return fmt.Errorf("profile %q max_runtime_seconds must be between 1 and 86400", p.Name)
	}
	if p.MaxResultRows <= 0 || p.MaxResultRows > 1000000 {
		return fmt.Errorf("profile %q max_result_rows must be between 1 and 1000000", p.Name)
	}
	if p.MaxResultBytes <= 0 || p.MaxResultBytes > 1073741824 {
		return fmt.Errorf("profile %q max_result_bytes must be between 1 and 1073741824", p.Name)
	}
	return nil
}

// Get returns the named profile, if loaded.
func (r *Registry) Get(name string) (Profile, bool) {
	p, ok := r.profiles[name]
	return p, ok
}

// List returns all loaded profiles sorted by name.
func (r *Registry) List() []Profile {
	names := make([]string, 0, len(r.profiles))
	for n := range r.profiles {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]Profile, 0, len(names))
	for _, n := range names {
		out = append(out, r.profiles[n])
	}
	return out
}
