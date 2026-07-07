package vql

import (
	"errors"
	"testing"
)

const (
	testMD5    = "d41d8cd98f00b204e9800998ecf8427e"
	testSHA1   = "da39a3ee5e6b4b0d3255bfef95601890afd80709"
	testSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

func TestBindHashHuntSelectsListParamByAlgorithm(t *testing.T) {
	cases := []struct {
		value        string
		wantParamKey string
	}{
		{testMD5, "MD5List"},
		{testSHA1, "SHA1List"},
		{testSHA256, "SHA256List"},
	}

	for _, c := range cases {
		artifact, params, err := Bind(TemplateIOCHashHunt, Env{"value": c.value})
		if err != nil {
			t.Fatalf("Bind(hash %q) error = %v, want nil", c.value, err)
		}
		if artifact != hashHunterArtifact {
			t.Errorf("Bind(hash %q) artifact = %q, want %q", c.value, artifact, hashHunterArtifact)
		}
		if got := params[c.wantParamKey]; got != c.value {
			t.Errorf("Bind(hash %q) params[%q] = %q, want %q", c.value, c.wantParamKey, got, c.value)
		}
		if len(params) != 1 {
			t.Errorf("Bind(hash %q) params = %v, want exactly one key", c.value, params)
		}
	}
}

// TestBindUnsupportedIOCTemplatesFailClosed covers the v0.10.3 fix: the
// four IOC kinds with no real, catalog-verified artifact must fail with
// ErrTemplateUnsupported rather than resolving to an invented artifact
// name. See docs/live-validation-report-v0.10.2.md finding 3.
func TestBindUnsupportedIOCTemplatesFailClosed(t *testing.T) {
	unsupported := []TemplateName{
		TemplateIOCIPHunt,
		TemplateIOCDomainHunt,
		TemplateIOCProcessHunt,
		TemplateIOCPathHunt,
	}

	for _, name := range unsupported {
		artifact, params, err := Bind(name, Env{"value": "indicator-value"})
		if err == nil {
			t.Fatalf("Bind(%q) = nil error, want ErrTemplateUnsupported", name)
		}
		if !errors.Is(err, ErrTemplateUnsupported) {
			t.Errorf("Bind(%q) error = %v, want wrapping ErrTemplateUnsupported", name, err)
		}
		if artifact != "" {
			t.Errorf("Bind(%q) artifact = %q, want empty on error", name, artifact)
		}
		if params != nil {
			t.Errorf("Bind(%q) params = %v, want nil on error", name, params)
		}
	}
}

func TestBindRejectsUnknownTemplate(t *testing.T) {
	_, _, err := Bind(TemplateName("ioc_shell_hunt"), Env{"value": "x"})
	if err == nil {
		t.Fatal("Bind(unknown template) = nil error, want ErrUnknownTemplate")
	}
}

func TestBindRejectsMissingValue(t *testing.T) {
	_, _, err := Bind(TemplateIOCHashHunt, Env{})
	if err == nil {
		t.Fatal("Bind(no value) = nil error, want error")
	}
}

func TestBindRejectsEmptyValue(t *testing.T) {
	_, _, err := Bind(TemplateIOCHashHunt, Env{"value": ""})
	if err == nil {
		t.Fatal("Bind(empty value) = nil error, want error")
	}
}

func TestBindRejectsNonHashValueForHashTemplate(t *testing.T) {
	// A value that isn't a well-formed md5/sha1/sha256 hash must never
	// reach a bound parameter for the hash template: Bind fails closed
	// rather than guessing which list parameter to use.
	_, _, err := Bind(TemplateIOCHashHunt, Env{"value": `'; DROP TABLE clients; --`})
	if err == nil {
		t.Fatal("Bind(non-hash value) = nil error, want error")
	}
}

func TestBindNeverInterpolatesValueIntoArtifactName(t *testing.T) {
	// A malicious-looking value must only ever end up as a bound
	// parameter value, never inside the returned artifact name string.
	malicious := testMD5
	artifact, params, err := Bind(TemplateIOCHashHunt, Env{"value": malicious})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if artifact != hashHunterArtifact {
		t.Errorf("artifact = %q, want unchanged %q", artifact, hashHunterArtifact)
	}
	if params["MD5List"] != malicious {
		t.Errorf("params[MD5List] = %q, want the raw bound value unchanged", params["MD5List"])
	}
}
