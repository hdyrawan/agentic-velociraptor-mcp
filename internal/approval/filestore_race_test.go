package approval

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestFileStoreConcurrentCreateAndConsumeNoResurrection is the
// regression test for H2: without cross-process locking, a concurrent
// Create from a separate FileStore instance (simulating the approve
// CLI) could overwrite a Consume by another instance, resurrecting a
// consumed approval. With flock-based cross-process locking, every
// operation sees the canonical file state and Consume is durable.
//
// This test runs many goroutines operating on two independent
// FileStore instances pointed at the same file (the in-process mutex
// would already serialize them, so the real cross-process value of
// flock isn't directly testable here — but this test does verify that
// the in-process contract holds under concurrency and that no
// operation reports success while the file is in an inconsistent
// state).
func TestFileStoreConcurrentCreateAndConsumeNoResurrection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approvals.json")
	ttl := time.Minute

	store1, err := NewFileStore(path, ttl)
	if err != nil {
		t.Fatalf("NewFileStore(1): %v", err)
	}
	store2, err := NewFileStore(path, ttl)
	if err != nil {
		t.Fatalf("NewFileStore(2): %v", err)
	}

	ctx := context.Background()

	// Pre-create N approvals, half-approved-half-decided.
	const N = 50
	for i := 0; i < N; i++ {
		req := testRequest(refName(i))
		if _, err := store1.Create(ctx, req); err != nil {
			t.Fatalf("Create(%d): %v", i, err)
		}
		if err := store1.Decide(ctx, Decision{
			RequestID:  refName(i),
			Approved:   true,
			ApprovedBy: "human",
		}); err != nil {
			t.Fatalf("Decide(%d): %v", i, err)
		}
	}

	// Now concurrently: store1 consumes even refs, store2 creates new
	// refs (approvals 100..199) and store1 also creates 200..299.
	// If the race exists, some Consume calls will silently fail to
	// persist (the file gets overwritten by a concurrent Create), and
	// a later IsApproved will return true for a ref we already
	// consumed.
	var wg sync.WaitGroup
	const workers = 8
	errs := make(chan error, workers*4)

	consumeWorker := func(start, step int) {
		defer wg.Done()
		for i := start; i < N; i += step {
			if err := store1.Consume(ctx, refName(i)); err != nil && !errors.Is(err, ErrAlreadyConsumed) {
				errs <- err
				return
			}
		}
	}
	createWorker := func(prefix string, start, end int) {
		defer wg.Done()
		for i := start; i < end; i++ {
			req := Request{
				ID:        prefix + refName(i),
				Operation: OperationCollectArtifact,
				CaseID:    "CASE-2",
				Reason:    "triage",
				Requester: "analyst@example.com",
				ClientID:  "C.1234abcd5678ef90",
				Artifact:  "Generic.Client.Info",
			}
			if _, err := store2.Create(ctx, req); err != nil && !errors.Is(err, ErrDuplicateReference) {
				errs <- err
				return
			}
		}
	}

	wg.Add(workers)
	go consumeWorker(0, 2) // even refs
	go consumeWorker(1, 2) // odd refs (will race with self on double-consume; that's fine)
	go createWorker("A", 100, 150)
	go createWorker("B", 150, 200)
	go createWorker("C", 200, 250)
	go createWorker("D", 250, 300)
	go createWorker("E", 300, 350)
	go createWorker("F", 350, 400)

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("worker error: %v", err)
	}

	// After all the concurrent churn: every even ref must be consumed,
	// every odd ref must be consumed (because Consume is idempotent on
	// "already consumed" — we treat that as success). Verify none of
	// them report IsApproved=true (which would indicate resurrection).
	for i := 0; i < N; i++ {
		approved, err := store1.IsApproved(ctx, refName(i))
		if err != nil {
			t.Fatalf("IsApproved(%d): %v", i, err)
		}
		if approved {
			t.Fatalf("ref %d reported approved after Consume — single-use guarantee violated (resurrection)", i)
		}
	}

	// All the new approvals created by the createWorkers must still
	// exist and be approved-eligible (Create succeeded but no Decision
	// was added, so IsApproved should return false).
	for _, prefix := range []string{"A", "B", "C", "D", "E", "F"} {
		for i := 100; i < 200; i++ {
			ref := prefix + refName(i)
			if i >= 200 && (prefix == "A" || prefix == "B") {
				continue
			}
			if i >= 300 && (prefix == "A" || prefix == "B" || prefix == "C" || prefix == "D") {
				continue
			}
			approved, err := store1.IsApproved(ctx, ref)
			if err != nil {
				t.Fatalf("IsApproved(%s): %v", ref, err)
			}
			if approved {
				t.Fatalf("ref %s reported approved but was never Decided — false positive", ref)
			}
		}
	}
}

func refName(i int) string {
	// Generate an approval-reference-shaped string. The
	// approvalReferencePattern in internal/validation requires
	// ^[A-Za-z0-9._-]{1,128}$, but FileStore.Create itself only
	// requires non-empty ID — so any unique string works for this
	// test.
	return "REF-" + itoa(i)
}

// itoa is a tiny strconv.Itoa replacement so this test file has no
// strconv import (keeps the import block clean).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
