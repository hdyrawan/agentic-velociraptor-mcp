package dfir

import (
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
		var p Profile
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("dfir: parse %s: %w", path, err)
		}
		if p.Name == "" {
			return nil, fmt.Errorf("dfir: %s: profile name is required", path)
		}
		if _, dup := profiles[p.Name]; dup {
			return nil, fmt.Errorf("dfir: duplicate profile name %q", p.Name)
		}
		profiles[p.Name] = p
	}

	return &Registry{profiles: profiles}, nil
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
