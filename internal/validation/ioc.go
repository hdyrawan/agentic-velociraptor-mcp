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
	IOCKindHash   IOCKind = "hash"
	IOCKindIP     IOCKind = "ip"
	IOCKindDomain IOCKind = "domain"
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
)

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
