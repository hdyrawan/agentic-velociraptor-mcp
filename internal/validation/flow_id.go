package validation

import (
	"fmt"
	"regexp"
)

// flowIDPattern matches Velociraptor flow identifiers. Velociraptor flow
// IDs use an F. prefix and an opaque URL-safe token. This validator is
// intentionally narrower than arbitrary text so handlers can reject
// malformed IDs before any Velociraptor call.
var flowIDPattern = regexp.MustCompile(`^F\.[A-Za-z0-9_-]{4,128}$`)

// FlowID validates a Velociraptor flow identifier string.
func FlowID(id string) error {
	if !flowIDPattern.MatchString(id) {
		return fmt.Errorf("validation: invalid flow id %q", id)
	}
	return nil
}
