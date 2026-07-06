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

// RotationConfig controls JSONLWriter file rotation. Zero values mean
// "no rotation"; that remains acceptable for local development but is
// not safe for production (see docs/production-deployment.md).
//
// Without rotation, a runaway or malicious MCP client can issue
// millions of tool calls and fill the disk holding audit.path. Once the
// disk fills, audit.Write fails; because approval-gated tools fail
// closed on a broken audit sink (see mcpserver.gateAuditForWrite),
// disk-fill becomes a denial-of-service against every write-capable
// operation. Rotation caps the disk footprint and prevents that.
type RotationConfig struct {
	// MaxSizeBytes rotates the audit file when it exceeds this size.
	// Zero disables size-based rotation (dev only).
	MaxSizeBytes int64
	// MaxFiles is the maximum number of rotated files (.1, .2, ...)
	// to retain. Zero defaults to 5.
	MaxFiles int
}

// JSONLWriter appends one JSON object per line to a file, opened in
// append-only mode. It is safe for concurrent use.
//
// When constructed via NewJSONLWriterWithRotation with a non-zero
// RotationConfig.MaxSizeBytes, the writer rotates the file in place
// when it exceeds the configured size, retaining up to
// RotationConfig.MaxFiles rotated copies (<path>.1, <path>.2, ...).
type JSONLWriter struct {
	mu       sync.Mutex
	file     *os.File
	enc      *json.Encoder
	path     string
	rotation RotationConfig
}

// NewJSONLWriter opens (creating if necessary) the file at path for
// append-only writes with 0600 permissions, since audit records may
// contain sensitive investigation metadata (case IDs, client IDs, IOCs)
// even after redaction of secrets. Rotation is disabled; use
// NewJSONLWriterWithRotation for production deployments.
func NewJSONLWriter(path string) (*JSONLWriter, error) {
	return NewJSONLWriterWithRotation(path, RotationConfig{})
}

// NewJSONLWriterWithRotation is like NewJSONLWriter but enables
// size-based rotation. A MaxSizeBytes of 0 disables rotation.
func NewJSONLWriterWithRotation(path string, rc RotationConfig) (*JSONLWriter, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	return &JSONLWriter{
		file:     f,
		enc:      json.NewEncoder(f),
		path:     path,
		rotation: rc,
	}, nil
}

// Write sanitizes fields is NOT performed here; callers must pass an
// already-sanitized Event. Write appends the JSON-encoded event followed
// by a newline, and is safe for concurrent callers. When rotation is
// configured and the file exceeds MaxSizeBytes, the file is rotated
// in-place before this call returns.
func (w *JSONLWriter) Write(evt Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.enc.Encode(evt); err != nil {
		return fmt.Errorf("audit: write event: %w", err)
	}
	if w.rotation.MaxSizeBytes > 0 {
		if info, err := w.file.Stat(); err == nil && info.Size() > w.rotation.MaxSizeBytes {
			w.rotateLocked()
		}
	}
	return nil
}

// rotateLocked closes the current file, renames it to <path>.1 (with
// older rotations shifted to .2, .3, ...), and opens a fresh file.
// Rotation errors are not surfaced on the Write path: a stuck rotation
// must not become a stuck audit pipeline, and the next write will
// simply append to the existing oversized file and try again. Rotation
// failures SHOULD, however, be made visible via stderr in a future
// revision — silently dropped audit events are themselves a security
// concern.
func (w *JSONLWriter) rotateLocked() {
	if err := w.file.Close(); err != nil {
		return
	}
	max := w.rotation.MaxFiles
	if max <= 0 {
		max = 5
	}
	// Shift older rotations up: .N-1 -> .N, ..., .1 -> .2.
	for i := max - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", w.path, i)
		dst := fmt.Sprintf("%s.%d", w.path, i+1)
		_ = os.Rename(src, dst)
	}
	// Current file -> .1
	_ = os.Rename(w.path, w.path+".1")
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	w.file = f
	w.enc = json.NewEncoder(f)
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
