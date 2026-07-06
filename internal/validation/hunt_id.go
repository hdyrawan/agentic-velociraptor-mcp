package validation

import (
	"fmt"
	"regexp"
)

// huntIDPattern matches Velociraptor hunt identifiers. Velociraptor hunt
// IDs use an H. prefix and an opaque URL-safe token, the same shape as
// flow IDs (see flow_id.go). This validator is intentionally narrower
// than arbitrary text so handlers can reject malformed IDs before any
// Velociraptor call.
var huntIDPattern = regexp.MustCompile(`^H\.[A-Za-z0-9_-]{4,128}$`)

// HuntID validates a Velociraptor hunt identifier string.
func HuntID(id string) error {
	if !huntIDPattern.MatchString(id) {
		return fmt.Errorf("validation: invalid hunt id %q", id)
	}
	return nil
}

// huntLabelPattern matches Velociraptor client labels used as hunt
// scopes: letters, digits, dot, dash, and underscore, 1-128 characters.
// Like every validator in this package it is an allowlist: whitespace,
// quotes, and every VQL/search-syntax-meaningful character are rejected
// by construction, so a label can never smuggle query syntax into a
// future client-search or hunt-condition call, an approval record, or an
// audit line.
var huntLabelPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// HuntLabel validates a client label used to scope a hunt.
func HuntLabel(label string) error {
	if !huntLabelPattern.MatchString(label) {
		return fmt.Errorf("validation: invalid hunt label %q", label)
	}
	return nil
}
