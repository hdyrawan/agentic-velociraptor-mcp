// Package vql holds allowlisted VQL template references and the safe
// parameter-binding mechanism used to fill them in.
//
// This package does not execute VQL and does not build VQL strings by
// concatenating caller-supplied text. Templates here are identified by
// name only; the actual VQL bodies live server-side as Velociraptor
// artifacts (see the artifact allowlist in config.PolicyConfig), which
// is the only place VQL text is authored and reviewed. Any caller input
// needed by a template is passed through Velociraptor's own parameter
// mechanism (equivalent to `SELECT * FROM Artifact.Name(param=value)`
// with `value` bound as a parameter, never interpolated into a query
// string) — see env.go and render.go.
//
// TODO(v0.1.0+): as tool handlers are implemented, confirm with a real
// Velociraptor server that parameter binding through the gRPC
// LaunchFlow/collect API is truly non-concatenative end to end (i.e.
// Velociraptor itself treats these as bound parameters, not text
// substituted into VQL before parsing). If any path turns out to require
// string interpolation, that path must be redesigned or dropped rather
// than shipped.
package vql

// TemplateName identifies a fixed, reviewed artifact/query template that
// a tool is allowed to invoke. This is intentionally not a free-form
// string: the stable core only ever references the constants below.
type TemplateName string

const (
	// TemplateIOCHashHunt corresponds to the ioc_hash_hunt DFIR profile:
	// hunts for a file hash across endpoints using a hash-lookup
	// artifact (e.g. Windows.Search.FileFinder / Generic.Forensic
	// hash-matching artifacts), with the hash passed as a bound
	// parameter.
	TemplateIOCHashHunt TemplateName = "ioc_hash_hunt"

	// TemplateIOCIPHunt corresponds to the ioc_ip_hunt DFIR profile:
	// hunts for network connections to/from a specific IP using
	// Windows.Network.Netstat or equivalent, with the IP passed as a
	// bound parameter.
	TemplateIOCIPHunt TemplateName = "ioc_ip_hunt"

	// TemplateIOCDomainHunt corresponds to the ioc_domain_hunt DFIR
	// profile: hunts for DNS/browser/network evidence of a domain, with
	// the domain passed as a bound parameter.
	TemplateIOCDomainHunt TemplateName = "ioc_domain_hunt"
)

// KnownTemplates lists every template the stable core is aware of. The
// IOC helper tool (velo_hunt_ioc_with_approval) must reject any
// TemplateName not present here.
var KnownTemplates = []TemplateName{
	TemplateIOCHashHunt,
	TemplateIOCIPHunt,
	TemplateIOCDomainHunt,
}
