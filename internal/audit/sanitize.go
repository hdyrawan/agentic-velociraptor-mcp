package audit

import "strings"

// DefaultRedactFields lists structured field/key names that must never
// appear in plaintext in an audit record or log line. This is the
// hard-coded floor; config.AuditConfig.RedactFields (from the operator's
// YAML) is additive to this list, never a replacement for it.
var DefaultRedactFields = []string{
	"client_private_key",
	"client_cert",
	"ca_certificate",
	"api_key",
	"approval_token",
	"password",
	"secret",
}

const redactedPlaceholder = "[REDACTED]"

// Sanitizer redacts sensitive values out of free-form maps before they
// are turned into an audit Event or written to any log output.
//
// TODO(v0.1.0-alpha.2): this is a placeholder shape. The real
// implementation must walk nested maps/structs (Velociraptor API config
// YAML, gRPC error payloads, VQL row results) recursively, matching key
// names case-insensitively, and must be unit-tested against fixtures
// containing real-shaped secrets (PEM blocks, JWT-like tokens) to make
// sure nothing leaks through a differently-cased or nested key.
type Sanitizer struct {
	redactKeys map[string]struct{}
}

// NewSanitizer builds a Sanitizer from the default redaction list plus
// any operator-configured extra field names.
func NewSanitizer(extra []string) *Sanitizer {
	keys := make(map[string]struct{}, len(DefaultRedactFields)+len(extra))
	for _, k := range DefaultRedactFields {
		keys[strings.ToLower(k)] = struct{}{}
	}
	for _, k := range extra {
		keys[strings.ToLower(k)] = struct{}{}
	}
	return &Sanitizer{redactKeys: keys}
}

// ShouldRedact reports whether a field name matches the redaction list.
func (s *Sanitizer) ShouldRedact(fieldName string) bool {
	_, ok := s.redactKeys[strings.ToLower(fieldName)]
	return ok
}

// SanitizeMap returns a shallow copy of m with any key matching the
// redaction list replaced by a fixed placeholder. It does not recurse
// into nested maps; see the TODO above.
func (s *Sanitizer) SanitizeMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s.ShouldRedact(k) {
			out[k] = redactedPlaceholder
			continue
		}
		out[k] = v
	}
	return out
}
