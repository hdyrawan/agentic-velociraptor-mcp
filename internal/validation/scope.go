package validation

import "fmt"

// HuntScope describes the intended targeting of a hunt or multi-client
// collection, prior to policy evaluation. It is deliberately narrow: an
// explicit client list or a label, never a free-form VQL WHERE clause.
type HuntScope struct {
	// ClientIDs, if non-empty, targets exactly these clients.
	ClientIDs []string

	// Label, if set, targets clients tagged with this label in
	// Velociraptor.
	Label string

	// All requests "every client". Must be explicitly rejected unless
	// policy.Engine.TargetAllAllowed() is true.
	All bool
}

// ValidateHuntScope checks structural validity (not policy eligibility):
// exactly one targeting mode must be set, any client IDs must be
// individually well-formed, and a label must match HuntLabel's
// allowlisted charset.
func ValidateHuntScope(s HuntScope) error {
	modes := 0
	if len(s.ClientIDs) > 0 {
		modes++
	}
	if s.Label != "" {
		modes++
	}
	if s.All {
		modes++
	}
	if modes != 1 {
		return fmt.Errorf("validation: hunt scope must set exactly one of client_ids, label, or all (got %d)", modes)
	}

	for _, id := range s.ClientIDs {
		if err := ClientID(id); err != nil {
			return err
		}
	}

	if s.Label != "" {
		if err := HuntLabel(s.Label); err != nil {
			return err
		}
	}

	return nil
}
