package validation

import "testing"

func TestClientID(t *testing.T) {
	valid := []string{"C.1234abcd5678ef90"}
	invalid := []string{"", "C.short", "not-a-client-id", "C.1234ABCD5678EF90", "C.1234abcd5678ef90; DROP"}

	for _, id := range valid {
		if err := ClientID(id); err != nil {
			t.Errorf("ClientID(%q) = %v, want nil", id, err)
		}
	}
	for _, id := range invalid {
		if err := ClientID(id); err == nil {
			t.Errorf("ClientID(%q) = nil, want error", id)
		}
	}
}

func TestArtifactName(t *testing.T) {
	valid := []string{"Generic.Client.Info", "Windows.System.Pslist"}
	invalid := []string{"", "generic", "Generic.Client.Info(); DROP", "Generic Client Info", "Generic.Client.Info'"}

	for _, n := range valid {
		if err := ArtifactName(n); err != nil {
			t.Errorf("ArtifactName(%q) = %v, want nil", n, err)
		}
	}
	for _, n := range invalid {
		if err := ArtifactName(n); err == nil {
			t.Errorf("ArtifactName(%q) = nil, want error", n)
		}
	}
}

func TestHash(t *testing.T) {
	cases := []struct {
		in       string
		wantKind string
		wantErr  bool
	}{
		{"d41d8cd98f00b204e9800998ecf8427e", "md5", false},
		{"da39a3ee5e6b4b0d3255bfef95601890afd80709", "sha1", false},
		{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b8551", "", true}, // 65 chars, invalid
		{"not-a-hash", "", true},
	}
	for _, c := range cases {
		kind, err := Hash(c.in)
		if c.wantErr && err == nil {
			t.Errorf("Hash(%q) = nil error, want error", c.in)
		}
		if !c.wantErr && (err != nil || kind != c.wantKind) {
			t.Errorf("Hash(%q) = (%q, %v), want (%q, nil)", c.in, kind, err, c.wantKind)
		}
	}
}

func TestIP(t *testing.T) {
	if err := IP("192.0.2.1"); err != nil {
		t.Errorf("IP(valid ipv4) = %v, want nil", err)
	}
	if err := IP("2001:db8::1"); err != nil {
		t.Errorf("IP(valid ipv6) = %v, want nil", err)
	}
	if err := IP("1[.]2[.]3[.]4"); err == nil {
		t.Error("IP(defanged) = nil, want error")
	}
	if err := IP("not-an-ip"); err == nil {
		t.Error("IP(garbage) = nil, want error")
	}
}

func TestDomain(t *testing.T) {
	valid := []string{"example.com", "sub.example.com", "example.com."}
	invalid := []string{"", "not a domain", "-example.com", "example.com; DROP"}

	for _, d := range valid {
		if err := Domain(d); err != nil {
			t.Errorf("Domain(%q) = %v, want nil", d, err)
		}
	}
	for _, d := range invalid {
		if err := Domain(d); err == nil {
			t.Errorf("Domain(%q) = nil, want error", d)
		}
	}
}

func TestSearchQuery(t *testing.T) {
	valid := []string{"", "WIN-HOST", "10.0.0.5", "label:triage", makeRepeated("a", 256)}
	for _, q := range valid {
		if err := SearchQuery(q); err != nil {
			t.Errorf("SearchQuery(%q) = %v, want nil", q, err)
		}
	}

	invalid := []string{"bad\x00query", "bad\nquery", makeRepeated("a", 257)}
	for _, q := range invalid {
		if err := SearchQuery(q); err == nil {
			t.Errorf("SearchQuery(%q) = nil, want error", q)
		}
	}
}

func makeRepeated(s string, n int) string {
	out := make([]byte, 0, n)
	for len(out) < n {
		out = append(out, s...)
	}
	return string(out[:n])
}

func TestValidateHuntScope(t *testing.T) {
	if err := ValidateHuntScope(HuntScope{ClientIDs: []string{"C.1234abcd5678ef90"}}); err != nil {
		t.Errorf("valid client_ids scope: %v", err)
	}
	if err := ValidateHuntScope(HuntScope{Label: "triage"}); err != nil {
		t.Errorf("valid label scope: %v", err)
	}
	if err := ValidateHuntScope(HuntScope{}); err == nil {
		t.Error("empty scope: want error, got nil")
	}
	if err := ValidateHuntScope(HuntScope{ClientIDs: []string{"C.1234abcd5678ef90"}, Label: "triage"}); err == nil {
		t.Error("multi-mode scope: want error, got nil")
	}
}
