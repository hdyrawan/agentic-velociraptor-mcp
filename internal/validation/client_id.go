// Package validation implements strict, allowlist-style input
// validation for every value that eventually flows into a Velociraptor
// gRPC call or a VQL parameter. Nothing here is a substitute for
// parameterized VQL (see internal/vql); it is an additional layer that
// rejects malformed or suspicious input before it ever reaches that
// layer.
package validation

import (
	"fmt"
	"regexp"
)

// clientIDPattern matches Velociraptor client identifiers, which are of
// the form "C." followed by 16 lowercase hex characters, e.g.
// "C.1234abcd5678ef90".
//
// TODO(v0.1.0-alpha.2): confirm this pattern against real client IDs
// returned by a live Velociraptor server once gRPC integration lands;
// adjust length/charset if needed rather than loosening to a permissive
// catch-all.
var clientIDPattern = regexp.MustCompile(`^C\.[0-9a-f]{16}$`)

// ClientID validates a Velociraptor client identifier string.
func ClientID(id string) error {
	if !clientIDPattern.MatchString(id) {
		return fmt.Errorf("validation: invalid client id %q", id)
	}
	return nil
}
