package vql

import "fmt"

// ErrUnknownTemplate is returned when a caller references a
// TemplateName not present in KnownTemplates.
var ErrUnknownTemplate = fmt.Errorf("vql: unknown template")

// Bind resolves a TemplateName plus an Env into the artifact name and
// bound-parameter map that internal/velociraptor should invoke. It does
// not build or return a VQL string at any point.
//
// TODO(v0.4.0): implement the real mapping from each TemplateName to its
// underlying allowlisted artifact name + expected parameter keys, once
// the corresponding server-side artifacts exist and are reviewed. Until
// then this always fails closed.
func Bind(name TemplateName, env Env) (artifact string, params map[string]string, err error) {
	known := false
	for _, t := range KnownTemplates {
		if t == name {
			known = true
			break
		}
	}
	if !known {
		return "", nil, fmt.Errorf("%w: %q", ErrUnknownTemplate, name)
	}
	return "", nil, fmt.Errorf("vql: template %q is not yet implemented", name)
}
