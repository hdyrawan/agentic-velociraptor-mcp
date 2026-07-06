package validation

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// IOCKind identifies which kind of indicator a value is expected to be,
// so it can be validated and later routed to the matching allowlisted
// VQL template (see internal/vql) with the right parameter name.
type IOCKind string

const (
	IOCKindHash    IOCKind = "hash"
	IOCKindIP      IOCKind = "ip"
	IOCKindDomain  IOCKind = "domain"
	IOCKindProcess IOCKind = "process"
	IOCKindPath    IOCKind = "path"
)

var (
	md5Pattern    = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
	sha1Pattern   = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
	sha256Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

	// domainPattern is deliberately strict: labels of letters, digits,
	// and hyphens, at least one dot, no leading/trailing hyphen. It
	// rejects anything containing whitespace, quotes, or VQL-meaningful
	// characters.
	domainPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

	// processNamePattern matches a bare process/image name (e.g.
	// "svchost.exe", "bash"), never a path: no "/" or "\" is in the
	// allowed set, so a path IOC must use IOCKindPath instead. The
	// charset excludes quotes, backticks, semicolons, and every other
	// VQL/shell-meaningful character by construction (allowlist, not
	// denylist).
	processNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,254}$`)

	// unixPathPattern / windowsPathPattern / uncPathPattern match
	// absolute filesystem paths only (never relative paths, which are
	// ambiguous as an indicator). Charsets are allowlists, same
	// rationale as processNamePattern.
	unixPathPattern    = regexp.MustCompile(`^/[A-Za-z0-9 ._-]*(/[A-Za-z0-9 ._-]+)*$`)
	windowsPathPattern = regexp.MustCompile(`^[A-Za-z]:\\[A-Za-z0-9 ._-]*(\\[A-Za-z0-9 ._-]+)*$`)
	uncPathPattern     = regexp.MustCompile(`^\\\\[A-Za-z0-9._-]+\\[A-Za-z0-9 ._-]+(\\[A-Za-z0-9 ._-]+)*$`)
)

// pathMaxLength bounds a path IOC's length (generous enough for deep
// Windows/Unix paths, far short of anything that could be a VQL/script
// payload smuggled in as a "path").
const pathMaxLength = 4096

// Hash validates that s is a well-formed MD5, SHA1, or SHA256 hex
// digest, and returns which it is.
func Hash(s string) (kind string, err error) {
	switch {
	case md5Pattern.MatchString(s):
		return "md5", nil
	case sha1Pattern.MatchString(s):
		return "sha1", nil
	case sha256Pattern.MatchString(s):
		return "sha256", nil
	default:
		return "", fmt.Errorf("validation: %q is not a valid md5/sha1/sha256 hash", s)
	}
}

// IP validates that s is a well-formed IPv4 or IPv6 address literal (no
// CIDR, no port, no surrounding text).
func IP(s string) error {
	if net.ParseIP(s) == nil {
		return fmt.Errorf("validation: invalid IP address %q", s)
	}
	return nil
}

// Domain validates that s is a syntactically well-formed DNS domain
// name.
func Domain(s string) error {
	s = strings.TrimSuffix(s, ".")
	if len(s) == 0 || len(s) > 253 || !domainPattern.MatchString(s) {
		return fmt.Errorf("validation: invalid domain %q", s)
	}
	return nil
}

// Process validates that s is a well-formed bare process/image name
// (e.g. "svchost.exe", "bash"). A path (even a relative one) must use
// Path instead.
func Process(s string) error {
	if !processNamePattern.MatchString(s) {
		return fmt.Errorf("validation: invalid process name %q", s)
	}
	return nil
}

// Path validates that s is a well-formed absolute filesystem path
// (Unix, Windows drive-letter, or Windows UNC), rejecting "..".
// traversal segments as a defense-in-depth measure even though this
// value is only ever bound as a Velociraptor query parameter, never
// dereferenced as a local path by this codebase.
func Path(s string) error {
	if s == "" || len(s) > pathMaxLength {
		return fmt.Errorf("validation: path is empty or exceeds %d characters", pathMaxLength)
	}
	if strings.Contains(s, "..") {
		return fmt.Errorf("validation: path %q must not contain '..' traversal segments", s)
	}
	if unixPathPattern.MatchString(s) || windowsPathPattern.MatchString(s) || uncPathPattern.MatchString(s) {
		return nil
	}
	return fmt.Errorf("validation: invalid file path %q (must be an absolute unix or windows path)", s)
}

// ValidateIOC validates value according to kind, dispatching to the
// kind-specific validator above. This is the single entry point
// velo_hunt_ioc_with_approval uses so an indicator's kind and value are
// always validated together, never independently.
func ValidateIOC(kind IOCKind, value string) error {
	switch kind {
	case IOCKindHash:
		_, err := Hash(value)
		return err
	case IOCKindIP:
		return IP(value)
	case IOCKindDomain:
		return Domain(value)
	case IOCKindProcess:
		return Process(value)
	case IOCKindPath:
		return Path(value)
	default:
		return fmt.Errorf("validation: unknown ioc kind %q", kind)
	}
}
