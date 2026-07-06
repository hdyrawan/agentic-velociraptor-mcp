package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/gofrs/flock"
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
// to a long-running MCP server process without a restart. Cross-process
// mutual exclusion is provided by an OS-level flock on a sibling
// `<path>.lock` file: without it, a concurrent `Create` from the
// approve CLI could silently overwrite a `Consume` by the MCP server,
// resurrecting a consumed approval (the single-use guarantee is the
// central security property of the approval system — see
// docs/approval-flow.md).
type FileStore struct {
	// mu serializes operations within the MCP server process. The
	// flock below handles cross-process serialization; this mutex is
	// belt-and-suspenders and makes the in-process contract explicit.
	mu   sync.Mutex
	path string
	ttl  time.Duration

	// lock provides cross-process mutual exclusion against the
	// approve CLI (a separate process sharing this store file). It is
	// acquired per-operation in withLock.
	lock *flock.Flock
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

	// Pre-create the file so the flock target exists and so we can
	// enforce 0600 from day one (see checkFilePermissionsLocked).
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			return nil, fmt.Errorf("approval: create store: %w", err)
		}
	}

	lockPath := path + ".lock"
	s := &FileStore{
		path: path,
		ttl:  ttl,
		lock: flock.New(lockPath),
	}

	// Acquire the cross-process lock once for the duration of
	// construction to do an initial permissions check; subsequent
	// operations re-acquire per call.
	if err := s.lock.Lock(); err != nil {
		return nil, fmt.Errorf("approval: lock store: %w", err)
	}
	defer s.lock.Unlock()

	if err := s.checkFilePermissionsLocked(); err != nil {
		return nil, err
	}

	// Initialize if the file is empty (e.g. touched by an operator
	// but never written).
	if info, err := os.Stat(path); err == nil && info.Size() == 0 {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			return nil, fmt.Errorf("approval: init store: %w", err)
		}
	}

	return s, nil
}

// withLock acquires the cross-process flock and the in-process mutex,
// then runs fn against a freshly-loaded record set. The in-process
// mutex is belt-and-suspenders: flock already serializes across
// processes, but the mutex makes the contract explicit for in-process
// callers and avoids a syscall storm under contention. The store file's
// permissions are re-verified on every load (see checkFilePermissionsLocked)
// so an accidental chmod between calls cannot silently expose approval
// state to other users on the host.
func (s *FileStore) withLock(ctx context.Context, fn func(records map[string]*record) error) error {
	// Honor context cancellation while waiting on the flock.
	lockCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		lockCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	if err := s.lock.TryLockContext(lockCtx, &flock.Options{}); err != nil {
		return fmt.Errorf("approval: acquire store lock: %w", err)
	}
	defer s.lock.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.checkFilePermissionsLocked(); err != nil {
		return err
	}

	records, err := s.load()
	if err != nil {
		return err
	}
	if err := fn(records); err != nil {
		return err
	}
	return nil
}

// checkFilePermissionsLocked enforces 0600 (or stricter) on the store
// file every time it's opened. This mirrors internal/velociraptor's
// LoadAPIConfig posture: an accidentally-relaxed mode bit on a file
// containing approval state (case IDs, client IDs, parameter values,
// decision history) is treated as a security defect, not a cosmetic
// one. Caller must hold the lock.
//
// POSIX-only: on Windows file mode bits don't carry the same semantics
// and are skipped rather than producing a permanently-failing check.
func (s *FileStore) checkFilePermissionsLocked() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(s.path)
	if err != nil {
		return fmt.Errorf("approval: stat store: %w", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Errorf(
			"approval: store %s: file mode %04o is too permissive (must be 0600 or stricter, e.g. chmod 0600)",
			s.path, perm)
	}
	return nil
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
	// fsync the temp file before rename so a crash between write and
	// rename doesn't leave a half-written file that the next reader
	// parses as the canonical state. Without this, "single-use" can
	// silently become "zero-use" (or worse, "indefinitely reusable")
	// on power loss.
	if f, err := os.Open(tmp); err == nil {
		_ = f.Sync()
		_ = f.Close()
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

	var created Request
	err := s.withLock(ctx, func(records map[string]*record) error {
		if _, exists := records[req.ID]; exists {
			return ErrDuplicateReference
		}

		if req.CreatedAt.IsZero() {
			req.CreatedAt = time.Now().UTC()
		}

		records[req.ID] = &record{Request: req}
		if err := s.save(records); err != nil {
			return err
		}
		created = req
		return nil
	})
	if err != nil {
		return Request{}, err
	}
	return created, nil
}

// Decide implements Store.
func (s *FileStore) Decide(ctx context.Context, dec Decision) error {
	if dec.ApprovedBy == "" {
		return ErrApprovedByRequired
	}

	return s.withLock(ctx, func(records map[string]*record) error {
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
	})
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
	var status Status
	err := s.withLock(ctx, func(records map[string]*record) error {
		var innerErr error
		status, innerErr = s.statusLocked(records, requestID)
		return innerErr
	})
	return status, err
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
	return s.withLock(ctx, func(records map[string]*record) error {
		rec, ok := records[requestID]
		if !ok {
			return ErrRequestNotFound
		}
		if rec.Consumed {
			return ErrAlreadyConsumed
		}
		rec.Consumed = true
		return s.save(records)
	})
}
