package vql

import (
	"fmt"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/validation"
)

// ErrUnknownTemplate is returned when a caller references a
// TemplateName not present in KnownTemplates.
var ErrUnknownTemplate = fmt.Errorf("vql: unknown template")

// ErrTemplateUnsupported is returned by Bind for a known TemplateName
// that has no real, safe, catalog-verified Velociraptor artifact backing
// it yet (see the v0.10.3 verification below). This is deliberately
// distinct from ErrUnknownTemplate: the template name itself is valid
// and recognized, but this project refuses to guess an artifact/
// parameter mapping it cannot confirm is real and safe. Callers
// (velo_hunt_ioc_with_approval, the `approve` CLI's BuildHuntIOCApprovalRequest
// path) must treat this as a hard failure that happens before any
// approval is consumed — see internal/mcpserver/tools_ioc.go, where
// Bind (via BuildHuntIOCApprovalRequest) is always called before
// verifyApproval/consumeApproval, so this error naturally leaves any
// referenced approval untouched.
var ErrTemplateUnsupported = fmt.Errorf("vql: unsupported until curated IOC artifacts are installed")

// hashHunterArtifact is Velocidex's own community "Generic.Detection.HashHunter"
// artifact — confirmed present in a real Velociraptor 0.76.3 server's
// catalog (disposable lab, 2026-07-07, same verification pass that
// produced docs/live-validation-report-v0.10.2.md finding 3). It takes
// one of three newline/whitespace-separated hash-list parameters
// (MD5List/SHA1List/SHA256List, selected by the indicator's own hash
// algorithm — see hashListParam below) and searches files under
// TargetGlob for a match, which is exactly this project's "hunt for a
// known-bad file hash across endpoints" use case. Every other field
// this artifact accepts (TargetGlob, Accessor, date/size filters) keeps
// its own server-side default; this project only ever binds the one
// hash-list parameter, never a VQL body.
const hashHunterArtifact = "Generic.Detection.HashHunter"

// hashListParam returns the Generic.Detection.HashHunter parameter name
// for a given hash's own algorithm, as already determined by
// validation.Hash (the same classification velo_hunt_ioc_with_approval's
// input validation already performed before this is ever called).
func hashListParam(value string) (string, error) {
	kind, err := validation.Hash(value)
	if err != nil {
		// Unreachable in practice: Bind is only ever called on an
		// already-validated value (validation.ValidateIOC ran first in
		// BuildHuntIOCApprovalRequest). Failing closed here rather than
		// panicking or guessing a parameter name.
		return "", fmt.Errorf("vql: cannot resolve hash list parameter: %w", err)
	}
	switch kind {
	case "md5":
		return "MD5List", nil
	case "sha1":
		return "SHA1List", nil
	case "sha256":
		return "SHA256List", nil
	default:
		return "", fmt.Errorf("vql: unrecognized hash kind %q", kind)
	}
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
//
// v0.10.3 status (docs/live-validation-report-v0.10.2.md finding 3):
// only TemplateIOCHashHunt has a real, catalog-verified artifact behind
// it. The pre-v0.10.3 mappings for IP/domain/process/path
// (System.IP.Hunt, System.Domain.Hunt, System.Process.Hunt,
// System.Path.Hunt) were illustrative placeholders confirmed, in a real
// 433-artifact Velociraptor 0.76.3 catalog, not to exist at all — a real
// CreateHunt call for any of them fails with "Unknown artifact ...".
// Rather than ship another guess, those four TemplateNames now fail
// closed with ErrTemplateUnsupported: no artifact/parameter is invented,
// and no real or curated equivalent for a generic, cross-platform
// "hunt for this IP/domain/process/path across endpoints" was found in
// the real catalog that fits this project's one-artifact-per-template
// model without misrepresenting coverage (e.g. an OS-specific artifact
// would silently produce no results for every other OS's clients,
// indistinguishable from "indicator not found"). Revisit if/when a
// reviewed, curated artifact set is installed specifically for this
// purpose — see KnownTemplates' doc comment.
func Bind(name TemplateName, env Env) (artifact string, params map[string]string, err error) {
	value, ok := env.Get(envValueKey)
	if !ok || value == "" {
		return "", nil, fmt.Errorf("vql: template %q requires a non-empty %q env value", name, envValueKey)
	}

	switch name {
	case TemplateIOCHashHunt:
		paramKey, err := hashListParam(value)
		if err != nil {
			return "", nil, err
		}
		return hashHunterArtifact, map[string]string{paramKey: value}, nil

	case TemplateIOCIPHunt, TemplateIOCDomainHunt, TemplateIOCProcessHunt, TemplateIOCPathHunt:
		return "", nil, fmt.Errorf("%w: ioc template %q", ErrTemplateUnsupported, name)

	default:
		return "", nil, fmt.Errorf("%w: %q", ErrUnknownTemplate, name)
	}
}
