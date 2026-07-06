package validation

import "testing"

func TestHuntID(t *testing.T) {
	valid := []string{"H.1234abcd5678ef90", "H.CCOB3JAM_x-1", "H.abcd"}
	for _, id := range valid {
		if err := HuntID(id); err != nil {
			t.Errorf("HuntID(%q) = %v, want nil", id, err)
		}
	}

	invalid := []string{
		"",
		"H.",
		"H.1",                // token shorter than 4
		"H.abc",              // token shorter than 4
		"F.1234abcd5678ef90", // flow prefix, not hunt
		"H.abc def",          // whitespace
		"H.'; SELECT 1",      // query-shaped
		"h.1234abcd5678ef90", // lowercase prefix
	}
	for _, id := range invalid {
		if err := HuntID(id); err == nil {
			t.Errorf("HuntID(%q) = nil, want error", id)
		}
	}
}

func TestHuntLabel(t *testing.T) {
	valid := []string{"windows", "linux", "dmz-servers", "tier_1.web", "A1"}
	for _, l := range valid {
		if err := HuntLabel(l); err != nil {
			t.Errorf("HuntLabel(%q) = %v, want nil", l, err)
		}
	}

	invalid := []string{
		"",
		"two words",
		"label\nall=true", // newline splice
		`label"quoted"`,
		"label;SELECT",
		"label:x", // Velociraptor search-syntax prefix form
		string(make([]byte, 129)),
	}
	for _, l := range invalid {
		if err := HuntLabel(l); err == nil {
			t.Errorf("HuntLabel(%q) = nil, want error", l)
		}
	}
}

func TestValidateHuntScopeRejectsBadLabel(t *testing.T) {
	if err := ValidateHuntScope(HuntScope{Label: "windows"}); err != nil {
		t.Errorf("valid label rejected: %v", err)
	}
	if err := ValidateHuntScope(HuntScope{Label: "windows\nall=true"}); err == nil {
		t.Error("label containing a newline splice was accepted")
	}
	if err := ValidateHuntScope(HuntScope{Label: "two words"}); err == nil {
		t.Error("label containing whitespace was accepted")
	}
}

// TestSingleLineFieldsRejectNewlines is the validation-layer half of the
// fingerprint delimiter-injection fix: identifiers and parameter values
// that participate in approval fingerprinting must not carry newlines,
// while genuinely multi-line free text (Reason) still may.
func TestSingleLineFieldsRejectNewlines(t *testing.T) {
	if err := CaseID("CASE-1\nclient=C.x"); err == nil {
		t.Error("CaseID with embedded newline was accepted")
	}
	if err := Requester("analyst\nother"); err == nil {
		t.Error("Requester with embedded newline was accepted")
	}
	if err := CollectionParameters(map[string]string{"k": "v\nparam:k2=v2"}); err == nil {
		t.Error("collection parameter value with embedded newline was accepted")
	}
	if err := CollectionParameters(map[string]string{"k\n2": "v"}); err == nil {
		t.Error("collection parameter key with embedded newline was accepted")
	}

	// Reason is operator-facing multi-line justification; newlines stay
	// legal there (it is deliberately excluded from the fingerprint).
	if err := Reason("line one\nline two"); err != nil {
		t.Errorf("multi-line Reason rejected: %v", err)
	}
	// Tabs remain acceptable in single-line fields.
	if err := CaseID("CASE\t1"); err != nil {
		t.Errorf("CaseID with tab rejected: %v", err)
	}
}
