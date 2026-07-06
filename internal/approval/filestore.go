package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// record is FileStore's on-disk representation of one Request/Decision
// pair.
type record struct {
	Request  Request   `json:"request"`
	Decision *Decision `json:"decision,omitempty"`
	Consumed bool      `json:"consumed"`
}

// FileStore is a Store backed by a single JSON file on disk. It exists
// so that approval decisions can be made by a human operator running a
// separate, non-MCP command (the agentic-velociraptor-mcp `approve` CLI
// subcommand) without that operator needing to share process memory
// with the running MCP server: the MCP server and the CLI both read and
// write the same file.
//
// FileStore re-reads the file on every call rather than caching state in
// memory, precisely so an external `approve` invocation becomes visible
// to a long-running MCP server process without a restart. This trades
// cross-process atomicity (two concurrent writers could in principle
// race) for operational simplicity appropriate to a single-analyst
// pilot; see docs/security-model.md's "known limitations" for this
// milestone.
type FileStore struct {
	mu   sync.Mutex
	path string
	ttl  time.Duration
}

// NewFileStore returns a FileStore persisting to path (created empty if
// it does not exist) with the given approval time-to-live, measured from
// Request.CreatedAt.
func NewFileStore(path string, ttl time.Duration) (*FileStore, error) {
	if path == "" {
		return nil, errors.New("approval: store path is required")
	}
	if ttl <= 0 {
		return nil, errors.New("approval: ttl must be > 0")
	}
	s := &FileStore{path: path, ttl: ttl}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := s.save(map[string]*record{}); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *FileStore) load() (map[string]*record, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]*record{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("approval: read store: %w", err)
	}
	if len(data) == 0 {
		return map[string]*record{}, nil
	}
	var records map[string]*record
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("approval: parse store: %w", err)
	}
	if records == nil {
		records = map[string]*record{}
	}
	return records, nil
}

func (s *FileStore) save(records map[string]*record) error {
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("approval: encode store: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("approval: write store: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("approval: rename store: %w", err)
	}
	return nil
}

// Create implements Store. req.ID is the caller-chosen approval
// reference; it must be non-empty and not already present in the store.
func (s *FileStore) Create(ctx context.Context, req Request) (Request, error) {
	if req.ID == "" {
		return Request{}, ErrReferenceRequired
	}
	if req.CaseID == "" {
		return Request{}, ErrCaseIDRequired
	}
	if req.Reason == "" {
		return Request{}, ErrReasonRequired
	}
	if req.Requester == "" {
		return Request{}, ErrRequesterRequired
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.load()
	if err != nil {
		return Request{}, err
	}
	if _, exists := records[req.ID]; exists {
		return Request{}, ErrDuplicateReference
	}

	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}

	records[req.ID] = &record{Request: req}
	if err := s.save(records); err != nil {
		return Request{}, err
	}
	return req, nil
}

// Decide implements Store.
func (s *FileStore) Decide(ctx context.Context, dec Decision) error {
	if dec.ApprovedBy == "" {
		return ErrApprovedByRequired
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.load()
	if err != nil {
		return err
	}
	rec, ok := records[dec.RequestID]
	if !ok {
		return ErrRequestNotFound
	}
	if rec.Decision != nil {
		return ErrAlreadyDecided
	}
	if dec.DecidedAt.IsZero() {
		dec.DecidedAt = time.Now().UTC()
	}
	rec.Decision = &dec
	return s.save(records)
}

func (s *FileStore) statusLocked(records map[string]*record, requestID string) (Status, error) {
	rec, ok := records[requestID]
	if !ok {
		return Status{}, ErrRequestNotFound
	}
	expired := time.Since(rec.Request.CreatedAt) > s.ttl
	return Status{Request: rec.Request, Decision: rec.Decision, Consumed: rec.Consumed, Expired: expired}, nil
}

// Get implements Store.
func (s *FileStore) Get(ctx context.Context, requestID string) (Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.load()
	if err != nil {
		return Status{}, err
	}
	return s.statusLocked(records, requestID)
}

// IsApproved implements Store.
func (s *FileStore) IsApproved(ctx context.Context, requestID string) (bool, error) {
	status, err := s.Get(ctx, requestID)
	if errors.Is(err, ErrRequestNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status.Consumed || status.Expired || status.Decision == nil {
		return false, nil
	}
	return status.Decision.Approved, nil
}

// Consume implements Store.
func (s *FileStore) Consume(ctx context.Context, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.load()
	if err != nil {
		return err
	}
	rec, ok := records[requestID]
	if !ok {
		return ErrRequestNotFound
	}
	if rec.Consumed {
		return ErrAlreadyConsumed
	}
	rec.Consumed = true
	return s.save(records)
}
