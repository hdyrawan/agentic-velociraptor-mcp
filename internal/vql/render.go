package vql

import "fmt"

// ErrUnknownTemplate is returned when a caller references a
// TemplateName not present in KnownTemplates.
var ErrUnknownTemplate = fmt.Errorf("vql: unknown template")

// templateBinding is the fixed, reviewed artifact name and parameter key
// one TemplateName resolves to.
//
// The artifact names below are illustrative, not confirmed against a
// real Velociraptor server or artifact catalog — the same honest caveat
// carried by profiles/ioc_hash_hunt.yaml, profiles/ioc_ip_hunt.yaml, and
// profiles/ioc_domain_hunt.yaml (see docs/dfir-profiles.md). What is
// real and safe here is the deterministic template-name -> (artifact,
// param key) mapping itself: it is pure Go data, involves no gRPC call,
// and guarantees the IOC value is always bound under the same fixed
// parameter name for a given template, never string-concatenated or
// caller-chosen. Whether "System.Hash.Hunt" (etc.) is the artifact a
// specific deployment's Velociraptor server actually exposes is an
// operator/deployment concern: the artifact allowlist
// (policy.allowed_artifacts) is where that gets confirmed, and the real
// gRPC hunt-start RPC remains scaffolded (velociraptor.HuntWriter.StartHunt
// returns ErrNotImplemented on grpcClient) until validated against a live
// lab; see docs/lab-validation-plan.md.
type templateBinding struct {
	artifact string
	paramKey string
}

var templateBindings = map[TemplateName]templateBinding{
	TemplateIOCHashHunt:    {artifact: "System.Hash.Hunt", paramKey: "HashValue"},
	TemplateIOCIPHunt:      {artifact: "System.IP.Hunt", paramKey: "IPAddress"},
	TemplateIOCDomainHunt:  {artifact: "System.Domain.Hunt", paramKey: "Domain"},
	TemplateIOCProcessHunt: {artifact: "System.Process.Hunt", paramKey: "ProcessName"},
	TemplateIOCPathHunt:    {artifact: "System.Path.Hunt", paramKey: "Path"},
}

// envValueKey is the single Env key every IOC template reads its
// indicator value from. Callers (velo_hunt_ioc_with_approval) always
// bind the already-validated indicator under this one key; Bind is what
// translates that generic key into the artifact-specific parameter name,
// so no caller ever needs to know or guess a template's real parameter
// name.
const envValueKey = "value"

// Bind resolves a TemplateName plus an Env into the artifact name and
// bound-parameter map that internal/velociraptor should invoke. It does
// not build or return a VQL string at any point, and it never echoes
// caller-supplied text into anything but a single, fixed-name parameter
// value.
func Bind(name TemplateName, env Env) (artifact string, params map[string]string, err error) {
	binding, known := templateBindings[name]
	if !known {
		return "", nil, fmt.Errorf("%w: %q", ErrUnknownTemplate, name)
	}

	value, ok := env.Get(envValueKey)
	if !ok || value == "" {
		return "", nil, fmt.Errorf("vql: template %q requires a non-empty %q env value", name, envValueKey)
	}

	return binding.artifact, map[string]string{binding.paramKey: value}, nil
}
