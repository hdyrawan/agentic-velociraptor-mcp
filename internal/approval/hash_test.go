package approval

import "testing"

// TestRequestFingerprintDeterministic proves identical requests hash
// identically regardless of Parameters/ClientIDs map and slice ordering.
func TestRequestFingerprintDeterministic(t *testing.T) {
	a := Request{
		Operation:  OperationStartHunt,
		CaseID:     "CASE-1",
		Artifact:   "Windows.System.Pslist",
		Parameters: map[string]string{"a": "1", "b": "2", "c": "3"},
		ClientIDs:  []string{"C.aaaaaaaaaaaaaaaa", "C.bbbbbbbbbbbbbbbb"},
	}
	b := Request{
		Operation:  OperationStartHunt,
		CaseID:     "CASE-1",
		Artifact:   "Windows.System.Pslist",
		Parameters: map[string]string{"c": "3", "b": "2", "a": "1"},
		ClientIDs:  []string{"C.bbbbbbbbbbbbbbbb", "C.aaaaaaaaaaaaaaaa"},
	}
	if RequestFingerprint(a) != RequestFingerprint(b) {
		t.Errorf("fingerprints differ for semantically identical requests")
	}

	// Reason/Requester are audit context, not targeting; they must not
	// affect the fingerprint.
	b.Reason = "different wording"
	b.Requester = "someone else"
	if RequestFingerprint(a) != RequestFingerprint(b) {
		t.Errorf("Reason/Requester changed the fingerprint; they must be excluded")
	}
}

// TestRequestFingerprintNewlineParameterSmuggling is the regression test
// for the delimiter-injection finding: under the old newline-delimited
// encoding, a single parameter whose value embeds "\nparam:k2=v2"
// serialized identically to two genuinely separate parameters, letting
// an approval for one parameter map be consumed by a request that binds
// a different map. The length-prefixed encoding must keep these
// distinct.
func TestRequestFingerprintNewlineParameterSmuggling(t *testing.T) {
	approved := Request{
		Operation:  OperationCollectArtifact,
		CaseID:     "CASE-1",
		ClientID:   "C.1234abcd5678ef90",
		Artifact:   "Windows.System.Pslist",
		Parameters: map[string]string{"k": "v", "k2": "v2"},
	}
	smuggled := Request{
		Operation:  OperationCollectArtifact,
		CaseID:     "CASE-1",
		ClientID:   "C.1234abcd5678ef90",
		Artifact:   "Windows.System.Pslist",
		Parameters: map[string]string{"k": "v\nparam:k2=v2"},
	}
	if RequestFingerprint(approved) == RequestFingerprint(smuggled) {
		t.Fatalf("parameter map with embedded newline splice collides with the approved parameter map")
	}
}

// TestRequestFingerprintNewlineFieldSmuggling proves a field value that
// embeds another field's serialized form cannot shift field boundaries:
// a CaseID of "A\nclient=C.x" must not collide with CaseID "A" plus
// ClientID "C.x", and a Label embedding "\nall=true" must not collide
// with a genuinely different TargetAll.
func TestRequestFingerprintNewlineFieldSmuggling(t *testing.T) {
	base := Request{
		Operation: OperationCancelFlow,
		CaseID:    "CASE-1",
		ClientID:  "C.1234abcd5678ef90",
		FlowID:    "F.ABCDEF",
	}
	spliced := Request{
		Operation: OperationCancelFlow,
		CaseID:    "CASE-1\nclient=C.1234abcd5678ef90",
		FlowID:    "F.ABCDEF",
	}
	if RequestFingerprint(base) == RequestFingerprint(spliced) {
		t.Fatalf("CaseID embedding a client= line collides with a separate ClientID field")
	}

	labelScoped := Request{
		Operation: OperationStartHunt,
		CaseID:    "CASE-1",
		Artifact:  "Windows.System.Pslist",
		Label:     "windows",
		TargetAll: false,
	}
	labelSpliced := Request{
		Operation: OperationStartHunt,
		CaseID:    "CASE-1",
		Artifact:  "Windows.System.Pslist",
		Label:     "windows\nall=false",
		TargetAll: false,
	}
	if RequestFingerprint(labelScoped) == RequestFingerprint(labelSpliced) {
		t.Fatalf("Label embedding an all= line collides with the plain label scope")
	}
}

// TestRequestFingerprintAdjacentFieldShift proves bytes cannot migrate
// between adjacent fields ("ab"+"c" vs "a"+"bc") and that an empty
// parameter map differs from one whose single key/value are empty.
func TestRequestFingerprintAdjacentFieldShift(t *testing.T) {
	a := Request{Operation: OperationCancelHunt, CaseID: "ab", HuntID: "c"}
	b := Request{Operation: OperationCancelHunt, CaseID: "a", HuntID: "bc"}
	if RequestFingerprint(a) == RequestFingerprint(b) {
		t.Fatalf("bytes shifted between adjacent fields collide")
	}

	none := Request{Operation: OperationStartHunt, CaseID: "x"}
	empty := Request{Operation: OperationStartHunt, CaseID: "x", Parameters: map[string]string{"": ""}}
	if RequestFingerprint(none) == RequestFingerprint(empty) {
		t.Fatalf("empty parameter map collides with map containing one empty key/value")
	}
}

// TestRequestFingerprintTargetingChangesHash spot-checks that every
// targeting field participates in the fingerprint.
func TestRequestFingerprintTargetingChangesHash(t *testing.T) {
	base := Request{
		Operation:  OperationStartHunt,
		CaseID:     "CASE-1",
		Artifact:   "Windows.System.Pslist",
		Parameters: map[string]string{"k": "v"},
		Label:      "windows",
	}
	fingerprint := RequestFingerprint(base)

	mutations := map[string]func(r *Request){
		"Operation":  func(r *Request) { r.Operation = OperationCancelHunt },
		"CaseID":     func(r *Request) { r.CaseID = "CASE-2" },
		"ClientID":   func(r *Request) { r.ClientID = "C.1234abcd5678ef90" },
		"Artifact":   func(r *Request) { r.Artifact = "Generic.Client.Info" },
		"Parameters": func(r *Request) { r.Parameters = map[string]string{"k": "other"} },
		"Profile":    func(r *Request) { r.Profile = "windows_basic_triage" },
		"HuntID":     func(r *Request) { r.HuntID = "H.1234abcd5678ef90" },
		"FlowID":     func(r *Request) { r.FlowID = "F.ABCDEF" },
		"UploadName": func(r *Request) { r.UploadName = "uploads/file.bin" },
		"Label":      func(r *Request) { r.Label = "linux" },
		"TargetAll":  func(r *Request) { r.TargetAll = true },
		"ClientIDs":  func(r *Request) { r.ClientIDs = []string{"C.aaaaaaaaaaaaaaaa"} },
	}
	for field, mutate := range mutations {
		mutated := base
		mutated.Parameters = map[string]string{"k": "v"}
		mutate(&mutated)
		if RequestFingerprint(mutated) == fingerprint {
			t.Errorf("changing %s did not change the fingerprint", field)
		}
	}
}
