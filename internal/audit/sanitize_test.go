package audit

import (
	"strings"
	"testing"
)

func TestSanitizeMapRedactsByKey(t *testing.T) {
	s := NewSanitizer(nil)
	in := map[string]string{
		"client_id":      "C.1234abcd5678ef90",
		"api_key":        "sk-abc",
		"client_cert":    "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n",
		"note":           "no secret here",
	}
	out := s.SanitizeMap(in)
	if out["api_key"] != redactedPlaceholder {
		t.Errorf("api_key not redacted: %q", out["api_key"])
	}
	if out["client_cert"] != redactedPlaceholder {
		t.Errorf("client_cert not redacted: %q", out["client_cert"])
	}
	if out["client_id"] != "C.1234abcd5678ef90" {
		t.Errorf("client_id was modified: %q", out["client_id"])
	}
	if out["note"] != "no secret here" {
		t.Errorf("note was modified: %q", out["note"])
	}
}

func TestSanitizeMapIsCaseInsensitive(t *testing.T) {
	s := NewSanitizer(nil)
	in := map[string]string{"API_KEY": "sk-abc", "ClientPrivateKey": "leaked"}
	out := s.SanitizeMap(in)
	if out["API_KEY"] != redactedPlaceholder {
		t.Errorf("API_KEY not redacted: %q", out["API_KEY"])
	}
	if out["ClientPrivateKey"] != redactedPlaceholder {
		t.Errorf("ClientPrivateKey not redacted: %q", out["ClientPrivateKey"])
	}
}

func TestSanitizeMapRespectsExtraFields(t *testing.T) {
	s := NewSanitizer([]string{"custom_secret"})
	in := map[string]string{"custom_secret": "v", "api_key": "k"}
	out := s.SanitizeMap(in)
	if out["custom_secret"] != redactedPlaceholder {
		t.Errorf("custom_secret not redacted: %q", out["custom_secret"])
	}
	if out["api_key"] != redactedPlaceholder {
		t.Errorf("api_key not redacted: %q", out["api_key"])
	}
}

func TestSanitizeStringStripsSinglePEMBlock(t *testing.T) {
	s := NewSanitizer(nil)
	in := "health check failed: bad cert -----BEGIN CERTIFICATE-----\nMIIB...\n-----END CERTIFICATE-----\n trailing"
	out := s.SanitizeString(in)
	if strings.Contains(out, "MIIB") {
		t.Errorf("PEM body not stripped: %q", out)
	}
	if strings.Contains(out, "BEGIN CERTIFICATE") {
		t.Errorf("PEM BEGIN marker not stripped: %q", out)
	}
	if !strings.Contains(out, "health check failed") {
		t.Errorf("non-PEM prefix lost: %q", out)
	}
	if !strings.Contains(out, "trailing") {
		t.Errorf("non-PEM suffix lost: %q", out)
	}
	if !strings.Contains(out, redactedPlaceholder) {
		t.Errorf("redaction placeholder missing: %q", out)
	}
}

// TestSanitizeStringStripsMultiplePEMBlocks is the regression test for
// M3: the original sanitizeTLSError only stripped the first PEM block
// in an error string, so a gRPC Status with multiple metadata entries
// containing PEM content would leak the second-and-later blocks.
func TestSanitizeStringStripsMultiplePEMBlocks(t *testing.T) {
	s := NewSanitizer(nil)
	in := "cert1 -----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----\n cert2 -----BEGIN CERTIFICATE-----\nBBB\n-----END CERTIFICATE-----\n end"
	out := s.SanitizeString(in)
	if strings.Contains(out, "AAA") || strings.Contains(out, "BBB") {
		t.Errorf("PEM body leaked: %q", out)
	}
	if strings.Count(out, redactedPlaceholder) != 2 {
		t.Errorf("expected 2 redaction placeholders, got %d in %q", strings.Count(out, redactedPlaceholder), out)
	}
	if !strings.Contains(out, "cert1") || !strings.Contains(out, "cert2") || !strings.Contains(out, "end") {
		t.Errorf("non-PEM text lost: %q", out)
	}
}

func TestSanitizeStringHandlesTruncatedPEM(t *testing.T) {
	s := NewSanitizer(nil)
	in := "bad -----BEGIN CERTIFICATE-----\nMIIB..."
	out := s.SanitizeString(in)
	if strings.Contains(out, "MIIB") {
		t.Errorf("truncated PEM body leaked: %q", out)
	}
	if !strings.Contains(out, redactedPlaceholder) {
		t.Errorf("placeholder missing: %q", out)
	}
}

func TestSanitizeStringNoPEMReturnsInputUnchanged(t *testing.T) {
	s := NewSanitizer(nil)
	in := "ordinary error message"
	if out := s.SanitizeString(in); out != in {
		t.Errorf("input modified: %q", out)
	}
}

// TestSanitizeAnyRecursesIntoNestedMaps is the regression test for H1:
// the original SanitizeMap was shallow, so a nested secret embedded
// under an innocent key name (e.g. an error payload's metadata map)
// survived into the audit log.
func TestSanitizeAnyRecursesIntoNestedMaps(t *testing.T) {
	s := NewSanitizer(nil)
	in := map[string]any{
		"outer": "kept",
		"meta": map[string]any{
			"client_private_key": "-----BEGIN RSA PRIVATE KEY-----\nLEAKED\n-----END RSA PRIVATE KEY-----\n",
			"nested": map[string]any{
				"api_key": "sk-nested",
				"safe":    "ok",
			},
		},
		"items": []any{
			"plain",
			map[string]any{"password": "p@ss"},
		},
	}
	out := s.SanitizeAny(in).(map[string]any)

	if out["outer"] != "kept" {
		t.Errorf("outer modified: %v", out["outer"])
	}
	meta := out["meta"].(map[string]any)
	if meta["client_private_key"] != redactedPlaceholder {
		t.Errorf("nested client_private_key not redacted: %v", meta["client_private_key"])
	}
	nested := meta["nested"].(map[string]any)
	if nested["api_key"] != redactedPlaceholder {
		t.Errorf("doubly-nested api_key not redacted: %v", nested["api_key"])
	}
	if nested["safe"] != "ok" {
		t.Errorf("doubly-nested safe value modified: %v", nested["safe"])
	}
	items := out["items"].([]any)
	item2 := items[1].(map[string]any)
	if item2["password"] != redactedPlaceholder {
		t.Errorf("slice-element password not redacted: %v", item2["password"])
	}
	if items[0] != "plain" {
		t.Errorf("slice plain element modified: %v", items[0])
	}
}

func TestSanitizeAnyRedactsStructByJsonTag(t *testing.T) {
	type cfg struct {
		Name             string `json:"name"`
		ClientPrivateKey string `json:"client_private_key"`
		APIKey           string `json:"api_key"`
	}
	s := NewSanitizer(nil)
	in := cfg{Name: "reader", ClientPrivateKey: "LEAKED", APIKey: "sk-1"}
	out := s.SanitizeAny(in).(map[string]any)
	if out["name"] != "reader" {
		t.Errorf("name modified: %v", out["name"])
	}
	if out["client_private_key"] != redactedPlaceholder {
		t.Errorf("client_private_key not redacted by json tag: %v", out["client_private_key"])
	}
	if out["api_key"] != redactedPlaceholder {
		t.Errorf("api_key not redacted by json tag: %v", out["api_key"])
	}
}

func TestSanitizeAnyRedactsPEMInsideStructStringValue(t *testing.T) {
	type resp struct {
		Error string `json:"error"`
	}
	s := NewSanitizer(nil)
	in := resp{Error: "tls: -----BEGIN CERTIFICATE-----\nAAA\n-----END CERTIFICATE-----\n trailing"}
	out := s.SanitizeAny(in).(map[string]any)
	errStr, ok := out["error"].(string)
	if !ok {
		t.Fatalf("error not a string: %v", out["error"])
	}
	if strings.Contains(errStr, "AAA") {
		t.Errorf("PEM body leaked through struct string field: %q", errStr)
	}
	if !strings.Contains(errStr, redactedPlaceholder) {
		t.Errorf("placeholder missing: %q", errStr)
	}
}

func TestSanitizeJSONStringReturnsValidJSON(t *testing.T) {
	s := NewSanitizer(nil)
	in := map[string]any{"api_key": "sk-1", "ok": "yes"}
	out := s.SanitizeJSONString(in)
	// Should be valid JSON: {"api_key":"[REDACTED]","ok":"yes"}
	if !strings.Contains(out, redactedPlaceholder) {
		t.Errorf("placeholder missing: %q", out)
	}
	if !strings.Contains(out, `"ok":"yes"`) {
		t.Errorf("non-sensitive field missing: %q", out)
	}
}
