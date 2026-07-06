package validation

import "testing"

func TestCaseID(t *testing.T) {
	valid := []string{"CASE-1234", "a"}
	invalid := []string{"", makeRepeated("a", 129), "bad\x00case"}

	for _, s := range valid {
		if err := CaseID(s); err != nil {
			t.Errorf("CaseID(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range invalid {
		if err := CaseID(s); err == nil {
			t.Errorf("CaseID(%q) = nil, want error", s)
		}
	}
}

func TestReason(t *testing.T) {
	if err := Reason(""); err == nil {
		t.Error("Reason(\"\") = nil, want error")
	}
	if err := Reason("triage requested by IR lead"); err != nil {
		t.Errorf("Reason(valid) = %v, want nil", err)
	}
	if err := Reason(makeRepeated("a", reasonMaxLength+1)); err == nil {
		t.Error("Reason(too long) = nil, want error")
	}
}

func TestRequester(t *testing.T) {
	if err := Requester(""); err == nil {
		t.Error("Requester(\"\") = nil, want error")
	}
	if err := Requester("analyst@example.com"); err != nil {
		t.Errorf("Requester(valid) = %v, want nil", err)
	}
}

func TestApprovalReference(t *testing.T) {
	valid := []string{"CASE-1234-01", "ref_1", "a.b-c_9"}
	invalid := []string{"", "has space", "has/slash", "has;semicolon", makeRepeated("a", 129)}

	for _, s := range valid {
		if err := ApprovalReference(s); err != nil {
			t.Errorf("ApprovalReference(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range invalid {
		if err := ApprovalReference(s); err == nil {
			t.Errorf("ApprovalReference(%q) = nil, want error", s)
		}
	}
}

func TestFlowID(t *testing.T) {
	valid := []string{"F.BN2HJC4N4T6KG", "F.abcd"}
	invalid := []string{"", "not-a-flow-id", "F.", "F.abc", "F.has space"}

	for _, s := range valid {
		if err := FlowID(s); err != nil {
			t.Errorf("FlowID(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range invalid {
		if err := FlowID(s); err == nil {
			t.Errorf("FlowID(%q) = nil, want error", s)
		}
	}
}

func TestUploadName(t *testing.T) {
	if err := UploadName(""); err == nil {
		t.Error("UploadName(\"\") = nil, want error")
	}
	if err := UploadName("uploads/file%20name.bin"); err != nil {
		t.Errorf("UploadName(valid) = %v, want nil", err)
	}
	if err := UploadName("bad\x00name"); err == nil {
		t.Error("UploadName(control char) = nil, want error")
	}
	if err := UploadName(makeRepeated("a", uploadNameMaxLength+1)); err == nil {
		t.Error("UploadName(too long) = nil, want error")
	}
}

func TestCollectionParameters(t *testing.T) {
	if err := CollectionParameters(nil); err != nil {
		t.Errorf("CollectionParameters(nil) = %v, want nil", err)
	}
	if err := CollectionParameters(map[string]string{"pid": "1234"}); err != nil {
		t.Errorf("CollectionParameters(valid) = %v, want nil", err)
	}
	if err := CollectionParameters(map[string]string{"": "1234"}); err == nil {
		t.Error("CollectionParameters(empty key) = nil, want error")
	}
	if err := CollectionParameters(map[string]string{"pid": "bad\x00value"}); err == nil {
		t.Error("CollectionParameters(control char value) = nil, want error")
	}
	if err := CollectionParameters(map[string]string{"pid": makeRepeated("a", maxParamValueLength+1)}); err == nil {
		t.Error("CollectionParameters(value too long) = nil, want error")
	}

	tooMany := make(map[string]string, maxCollectionParams+1)
	for i := 0; i < maxCollectionParams+1; i++ {
		tooMany[makeRepeated("k", 1)+string(rune('a'+i))] = "v"
	}
	if err := CollectionParameters(tooMany); err == nil {
		t.Error("CollectionParameters(too many) = nil, want error")
	}
}
