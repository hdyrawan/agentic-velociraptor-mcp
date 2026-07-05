package velociraptor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const validAPIConfigYAML = `
ca_certificate: |
  -----BEGIN CERTIFICATE-----
  fake-ca-cert-not-a-real-secret
  -----END CERTIFICATE-----
client_cert: |
  -----BEGIN CERTIFICATE-----
  fake-client-cert-not-a-real-secret
  -----END CERTIFICATE-----
client_private_key: |
  -----BEGIN RSA PRIVATE KEY-----
  fake-private-key-not-a-real-secret
  -----END RSA PRIVATE KEY-----
api_connection_string: 127.0.0.1:8001
name: test-reader
`

func writeAPIConfig(t *testing.T, dir, content string, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, "reader.api.config.yaml")
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatalf("write api config fixture: %v", err)
	}
	return path
}

func TestLoadAPIConfigMissingFile(t *testing.T) {
	_, err := LoadAPIConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("LoadAPIConfig: expected error for missing file, got nil")
	}
}

func TestLoadAPIConfigEmptyPath(t *testing.T) {
	_, err := LoadAPIConfig("")
	if err == nil {
		t.Fatal("LoadAPIConfig: expected error for empty path, got nil")
	}
}

func TestLoadAPIConfigRejectsDirectory(t *testing.T) {
	_, err := LoadAPIConfig(t.TempDir())
	if err == nil {
		t.Fatal("LoadAPIConfig: expected error when path is a directory, got nil")
	}
}

func TestLoadAPIConfigRejectsOverlyPermissiveMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits are not meaningful on windows")
	}
	dir := t.TempDir()
	path := writeAPIConfig(t, dir, validAPIConfigYAML, 0o644)

	_, err := LoadAPIConfig(path)
	if err == nil {
		t.Fatal("LoadAPIConfig: expected error for group/other-readable file, got nil")
	}
	if strings.Contains(err.Error(), "BEGIN") {
		t.Errorf("error message leaks certificate content: %v", err)
	}
}

func TestLoadAPIConfigMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	path := writeAPIConfig(t, dir, "name: incomplete\n", 0o600)

	_, err := LoadAPIConfig(path)
	if err == nil {
		t.Fatal("LoadAPIConfig: expected error for missing required fields, got nil")
	}
}

func TestLoadAPIConfigValid(t *testing.T) {
	dir := t.TempDir()
	path := writeAPIConfig(t, dir, validAPIConfigYAML, 0o600)

	cfg, err := LoadAPIConfig(path)
	if err != nil {
		t.Fatalf("LoadAPIConfig: %v", err)
	}
	if cfg.APIConnectionString != "127.0.0.1:8001" {
		t.Errorf("APIConnectionString = %q, want %q", cfg.APIConnectionString, "127.0.0.1:8001")
	}
	if cfg.Name != "test-reader" {
		t.Errorf("Name = %q, want %q", cfg.Name, "test-reader")
	}
	if !strings.Contains(cfg.CACertificate, "BEGIN CERTIFICATE") {
		t.Error("CACertificate was not parsed")
	}
}

func TestAPIConfigStringDoesNotLeakSecrets(t *testing.T) {
	cfg := APIConfig{
		CACertificate:    "-----BEGIN CERTIFICATE-----\nsecret\n-----END CERTIFICATE-----",
		ClientCert:       "-----BEGIN CERTIFICATE-----\nsecret\n-----END CERTIFICATE-----",
		ClientPrivateKey: "-----BEGIN RSA PRIVATE KEY-----\nsecret\n-----END RSA PRIVATE KEY-----",
	}

	if strings.Contains(cfg.String(), "secret") || strings.Contains(cfg.String(), "BEGIN") {
		t.Errorf("String() leaks secret content: %v", cfg.String())
	}
	if strings.Contains(cfg.GoString(), "secret") || strings.Contains(cfg.GoString(), "BEGIN") {
		t.Errorf("GoString() leaks secret content: %v", cfg.GoString())
	}
}
