package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// Sink writes Events somewhere durable. The stable core only ships a
// JSONLWriter, but tool handlers should depend on this interface so
// tests can substitute an in-memory sink.
type Sink interface {
	Write(Event) error
	Close() error
}

// JSONLWriter appends one JSON object per line to a file, opened in
// append-only mode. It is safe for concurrent use.
//
// TODO(v0.1.0-alpha.2): decide on rotation / max file size policy before
// this is used against a real deployment; unbounded append-only growth
// is acceptable for the skeleton but not for production hardening
// (v0.5.0).
type JSONLWriter struct {
	mu   sync.Mutex
	file *os.File
	enc  *json.Encoder
}

// NewJSONLWriter opens (creating if necessary) the file at path for
// append-only writes with 0600 permissions, since audit records may
// contain sensitive investigation metadata (case IDs, client IDs, IOCs)
// even after redaction of secrets.
func NewJSONLWriter(path string) (*JSONLWriter, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	return &JSONLWriter{file: f, enc: json.NewEncoder(f)}, nil
}

// Write sanitizes fields is NOT performed here; callers must pass an
// already-sanitized Event. Write appends the JSON-encoded event followed
// by a newline, and is safe for concurrent callers.
func (w *JSONLWriter) Write(evt Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.enc.Encode(evt); err != nil {
		return fmt.Errorf("audit: write event: %w", err)
	}
	return nil
}

// Close closes the underlying file.
func (w *JSONLWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// NopSink discards events. Used when audit.enabled is false, which
// should only ever happen in local development, never production.
type NopSink struct{}

func (NopSink) Write(Event) error { return nil }
func (NopSink) Close() error      { return nil }
