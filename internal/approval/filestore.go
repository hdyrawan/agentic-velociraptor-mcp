package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// lockRetryDelay is how often TryLockContext retries acquiring the
// cross-process store lock while waiting on lockCtx (bounded by a
// 5-second default timeout in withLock, or the caller's own context
// deadline).
const lockRetryDelay = 25 * time.Millisecond

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
	// mu serializes operations within the MCP server process, and must
	// be acquired before the flock below (see withLock's doc comment for
	// why the order matters).
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

// withLock acquires the in-process mutex and the cross-process flock, in
// that order, then runs fn against a freshly-loaded record set.
//
// The mutex must be acquired first, not the flock: gofrs/flock's
// *Flock.TryLock is per-*Flock-instance, not per-call. If goroutine A
// already holds this FileStore's flock, a concurrent call to TryLock
// from goroutine B on the *same* FileStore (and therefore the same
// *flock.Flock) sees its own already-locked bookkeeping (Flock.l) and
// returns a spurious immediate success, without ever touching the OS
// lock or waiting for A to finish. Locking the flock before the mutex
// therefore only protects against a genuinely different FileStore
// instance (a separate process, or a separate instance in the same
// process); two goroutines sharing one FileStore could both believe
// they hold the lock, with only the mutex actually serializing them —
// and a real cross-instance writer could slip in through the underlying
// OS lock during the gap between A's real unlock and B's belated,
// mutex-only-protected critical section. This was a real, reproducible
// bug (intermittent lost updates / consumed-approval resurrection under
// concurrent same-instance callers; see
// TestFileStoreConcurrentCreateAndConsumeNoResurrection).
//
// Acquiring the mutex first guarantees only one goroutine per instance
// ever attempts the flock at a time, so TryLock's per-instance shortcut
// can never fire while another goroutine of the same instance still
// needs the real OS-level lock.
//
// The store file's permissions are re-verified on every load (see
// checkFilePermissionsLocked) so an accidental chmod between calls
// cannot silently expose approval state to other users on the host.
func (s *FileStore) withLock(ctx context.Context, fn func(records map[string]*record) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Honor context cancellation while waiting on the flock.
	lockCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		lockCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}
	locked, err := s.lock.TryLockContext(lockCtx, lockRetryDelay)
	if err != nil {
		return fmt.Errorf("approval: acquire store lock: %w", err)
	}
	if !locked {
		return fmt.Errorf("approval: acquire store lock: %w", lockCtx.Err())
	}
	defer s.lock.Unlock()

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
	// The temp file name must be unique per call, not a fixed
	// "<path>.tmp": two independent FileStore instances pointed at the
	// same path (e.g. the MCP server and a concurrently-running approve
	// CLI, as in TestFileStoreConcurrentCreateAndConsumeNoResurrection)
	// each hold their own *flock.Flock file descriptor, and a shared
	// fixed temp name would let one instance's rename consume (or race
	// against) another instance's in-progress write even though the
	// flock itself correctly serializes the read-modify-write of the
	// canonical store file.
	tmpFile, err := os.CreateTemp(filepath.Dir(s.path), filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("approval: create temp store file: %w", err)
	}
	tmp := tmpFile.Name()
	defer os.Remove(tmp) // no-op once renamed away below

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("approval: write store: %w", err)
	}
	if err := tmpFile.Chmod(0o600); err != nil {
		tmpFile.Close()
		return fmt.Errorf("approval: chmod store: %w", err)
	}
	// fsync the temp file before rename so a crash between write and
	// rename doesn't leave a half-written file that the next reader
	// parses as the canonical state. Without this, "single-use" can
	// silently become "zero-use" (or worse, "indefinitely reusable")
	// on power loss.
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("approval: sync store: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("approval: close store: %w", err)
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
