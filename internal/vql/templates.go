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
	// hunts for a file hash across endpoints. As of v0.10.3, Bind resolves
	// this to Generic.Detection.HashHunter — confirmed present in a real
	// Velociraptor 0.76.3 catalog (see render.go) — with the hash bound
	// to whichever of its MD5List/SHA1List/SHA256List parameters matches
	// the indicator's own algorithm. This is the one IOC kind with a
	// real, curated, catalog-verified artifact behind it.
	TemplateIOCHashHunt TemplateName = "ioc_hash_hunt"

	// TemplateIOCIPHunt corresponds to the ioc_ip_hunt DFIR profile:
	// intended to hunt for network connections to/from a specific IP.
	// As of v0.10.3, Bind returns ErrTemplateUnsupported for this
	// template — no real, catalog-verified, cross-platform "hunt by IP"
	// artifact was found in a real Velociraptor 0.76.3 catalog that fits
	// this project's one-artifact-per-template model; the pre-v0.10.3
	// "System.IP.Hunt" mapping was confirmed not to exist at all (see
	// docs/live-validation-report-v0.10.2.md finding 3).
	TemplateIOCIPHunt TemplateName = "ioc_ip_hunt"

	// TemplateIOCDomainHunt corresponds to the ioc_domain_hunt DFIR
	// profile: intended to hunt for DNS/browser/network evidence of a
	// domain. As of v0.10.3, Bind returns ErrTemplateUnsupported for the
	// same reason as TemplateIOCIPHunt — no real matching built-in
	// artifact was found.
	TemplateIOCDomainHunt TemplateName = "ioc_domain_hunt"

	// TemplateIOCProcessHunt is intended to hunt for a running process/
	// image name across endpoints. As of v0.10.3, Bind returns
	// ErrTemplateUnsupported: the real process-listing artifacts
	// (Windows.System.Pslist, Linux.Sys.Pslist, ...) take no
	// process-name filter parameter, so binding an indicator to one of
	// them would silently ignore it and return every process — a false
	// "hunt", not a real one.
	TemplateIOCProcessHunt TemplateName = "ioc_process_hunt"

	// TemplateIOCPathHunt is intended to hunt for evidence of a specific
	// filesystem path across endpoints. As of v0.10.3, Bind returns
	// ErrTemplateUnsupported: real per-OS FileFinder artifacts exist
	// (Windows.Search.FileFinder, Linux.Search.FileFinder,
	// MacOS.Search.FileFinder) and do accept a path/glob parameter, but
	// this project's one-artifact-per-template model has no mechanism to
	// pick the right one per client OS — binding to a single OS's
	// FileFinder would silently produce no results for every other OS's
	// clients, indistinguishable from "path not found" there.
	TemplateIOCPathHunt TemplateName = "ioc_path_hunt"
)

// KnownTemplates lists every template the stable core is aware of. The
// IOC helper tool (velo_hunt_ioc_with_approval) must reject any
// TemplateName not present here. Being "known" only means the name is
// recognized — see each constant's doc comment above and Bind's doc
// comment in render.go for which of these actually resolve to a real
// artifact (only TemplateIOCHashHunt, as of v0.10.3) versus fail closed
// with ErrTemplateUnsupported.
var KnownTemplates = []TemplateName{
	TemplateIOCHashHunt,
	TemplateIOCIPHunt,
	TemplateIOCDomainHunt,
	TemplateIOCProcessHunt,
	TemplateIOCPathHunt,
}
