package vql

// Env is the set of caller-supplied parameters bound into a template or
// artifact invocation. Every value here must have already passed
// internal/validation (e.g. validation.Hash, validation.IP,
// validation.Domain) before being placed in an Env; this package does
// not re-validate, it only carries already-validated values through to
// the Velociraptor call as bound parameters.
type Env map[string]string

// Get returns a parameter value and whether it was present.
func (e Env) Get(key string) (string, bool) {
	v, ok := e[key]
	return v, ok
}
