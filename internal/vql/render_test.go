package vql

import "testing"

func TestBindKnownTemplates(t *testing.T) {
	cases := []struct {
		name         TemplateName
		wantArtifact string
		wantParamKey string
	}{
		{TemplateIOCHashHunt, "System.Hash.Hunt", "HashValue"},
		{TemplateIOCIPHunt, "System.IP.Hunt", "IPAddress"},
		{TemplateIOCDomainHunt, "System.Domain.Hunt", "Domain"},
		{TemplateIOCProcessHunt, "System.Process.Hunt", "ProcessName"},
		{TemplateIOCPathHunt, "System.Path.Hunt", "Path"},
	}

	for _, c := range cases {
		artifact, params, err := Bind(c.name, Env{"value": "indicator-value"})
		if err != nil {
			t.Fatalf("Bind(%q) error = %v, want nil", c.name, err)
		}
		if artifact != c.wantArtifact {
			t.Errorf("Bind(%q) artifact = %q, want %q", c.name, artifact, c.wantArtifact)
		}
		if got := params[c.wantParamKey]; got != "indicator-value" {
			t.Errorf("Bind(%q) params[%q] = %q, want %q", c.name, c.wantParamKey, got, "indicator-value")
		}
		if len(params) != 1 {
			t.Errorf("Bind(%q) params = %v, want exactly one key", c.name, params)
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

func TestBindNeverInterpolatesValueIntoArtifactName(t *testing.T) {
	// A malicious-looking value must only ever end up as a bound
	// parameter value, never inside the returned artifact name string.
	malicious := `'; DROP TABLE clients; --`
	artifact, params, err := Bind(TemplateIOCHashHunt, Env{"value": malicious})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if artifact != "System.Hash.Hunt" {
		t.Errorf("artifact = %q, want unchanged System.Hash.Hunt", artifact)
	}
	if params["HashValue"] != malicious {
		t.Errorf("params[HashValue] = %q, want the raw bound value unchanged", params["HashValue"])
	}
}
