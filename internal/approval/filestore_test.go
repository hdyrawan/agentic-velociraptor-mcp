package approval

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *FileStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "approvals.json")
	store, err := NewFileStore(path, time.Minute)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return store
}

func testRequest(id string) Request {
	return Request{
		ID:        id,
		Operation: OperationCollectArtifact,
		CaseID:    "CASE-1",
		Reason:    "triage",
		Requester: "analyst@example.com",
		ClientID:  "C.1234abcd5678ef90",
		Artifact:  "Generic.Client.Info",
	}
}

func TestFileStoreCreateRejectsMissingFields(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	cases := []struct {
		name string
		req  Request
		want error
	}{
		{"missing id", Request{CaseID: "c", Reason: "r", Requester: "u"}, ErrReferenceRequired},
		{"missing case", Request{ID: "ref-1", Reason: "r", Requester: "u"}, ErrCaseIDRequired},
		{"missing reason", Request{ID: "ref-1", CaseID: "c", Requester: "u"}, ErrReasonRequired},
		{"missing requester", Request{ID: "ref-1", CaseID: "c", Reason: "r"}, ErrRequesterRequired},
	}
	for _, tc := range cases {
		if _, err := store.Create(ctx, tc.req); !errors.Is(err, tc.want) {
			t.Errorf("%s: Create error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

func TestFileStoreCreateRejectsDuplicateReference(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Create(ctx, testRequest("ref-1")); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if _, err := store.Create(ctx, testRequest("ref-1")); !errors.Is(err, ErrDuplicateReference) {
		t.Fatalf("duplicate Create error = %v, want ErrDuplicateReference", err)
	}
}

func TestFileStoreDecideRequiresApprovedBy(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.Create(ctx, testRequest("ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Decide(ctx, Decision{RequestID: "ref-1", Approved: true}); !errors.Is(err, ErrApprovedByRequired) {
		t.Fatalf("Decide error = %v, want ErrApprovedByRequired", err)
	}
}

func TestFileStoreDecideRejectsUnknownReference(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	err := store.Decide(ctx, Decision{RequestID: "does-not-exist", Approved: true, ApprovedBy: "human"})
	if !errors.Is(err, ErrRequestNotFound) {
		t.Fatalf("Decide error = %v, want ErrRequestNotFound", err)
	}
}

func TestFileStoreDecideRejectsRedecision(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.Create(ctx, testRequest("ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Decide(ctx, Decision{RequestID: "ref-1", Approved: true, ApprovedBy: "human"}); err != nil {
		t.Fatalf("first Decide: %v", err)
	}
	if err := store.Decide(ctx, Decision{RequestID: "ref-1", Approved: false, ApprovedBy: "human"}); !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("second Decide error = %v, want ErrAlreadyDecided", err)
	}
}

func TestFileStoreIsApprovedLifecycle(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Unknown reference: false, no error.
	approved, err := store.IsApproved(ctx, "unknown")
	if err != nil || approved {
		t.Fatalf("IsApproved(unknown) = (%v, %v), want (false, nil)", approved, err)
	}

	if _, err := store.Create(ctx, testRequest("ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Created but not decided: false.
	if approved, err := store.IsApproved(ctx, "ref-1"); err != nil || approved {
		t.Fatalf("IsApproved(undecided) = (%v, %v), want (false, nil)", approved, err)
	}

	if err := store.Decide(ctx, Decision{RequestID: "ref-1", Approved: true, ApprovedBy: "human"}); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	// Decided and approved: true.
	if approved, err := store.IsApproved(ctx, "ref-1"); err != nil || !approved {
		t.Fatalf("IsApproved(approved) = (%v, %v), want (true, nil)", approved, err)
	}

	if err := store.Consume(ctx, "ref-1"); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	// Consumed: false.
	if approved, err := store.IsApproved(ctx, "ref-1"); err != nil || approved {
		t.Fatalf("IsApproved(consumed) = (%v, %v), want (false, nil)", approved, err)
	}
}

func TestFileStoreIsApprovedFalseWhenDenied(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.Create(ctx, testRequest("ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Decide(ctx, Decision{RequestID: "ref-1", Approved: false, ApprovedBy: "human", Note: "not justified"}); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if approved, err := store.IsApproved(ctx, "ref-1"); err != nil || approved {
		t.Fatalf("IsApproved(denied) = (%v, %v), want (false, nil)", approved, err)
	}
}

func TestFileStoreConsumeRejectsDoubleUse(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.Create(ctx, testRequest("ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Decide(ctx, Decision{RequestID: "ref-1", Approved: true, ApprovedBy: "human"}); err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if err := store.Consume(ctx, "ref-1"); err != nil {
		t.Fatalf("first Consume: %v", err)
	}
	if err := store.Consume(ctx, "ref-1"); !errors.Is(err, ErrAlreadyConsumed) {
		t.Fatalf("second Consume error = %v, want ErrAlreadyConsumed", err)
	}
}

func TestFileStoreExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approvals.json")
	store, err := NewFileStore(path, time.Millisecond)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	ctx := context.Background()
	if _, err := store.Create(ctx, testRequest("ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Decide(ctx, Decision{RequestID: "ref-1", Approved: true, ApprovedBy: "human"}); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	status, err := store.Get(ctx, "ref-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !status.Expired {
		t.Fatal("status.Expired = false, want true")
	}
	if approved, err := store.IsApproved(ctx, "ref-1"); err != nil || approved {
		t.Fatalf("IsApproved(expired) = (%v, %v), want (false, nil)", approved, err)
	}
}

func TestFileStorePersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approvals.json")
	ctx := context.Background()

	store1, err := NewFileStore(path, time.Minute)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	if _, err := store1.Create(ctx, testRequest("ref-1")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store1.Decide(ctx, Decision{RequestID: "ref-1", Approved: true, ApprovedBy: "human"}); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	// A second, independent FileStore instance pointed at the same path
	// (simulating the separate `approve` CLI process) must see the
	// decision.
	store2, err := NewFileStore(path, time.Minute)
	if err != nil {
		t.Fatalf("NewFileStore (second instance): %v", err)
	}
	if approved, err := store2.IsApproved(ctx, "ref-1"); err != nil || !approved {
		t.Fatalf("IsApproved from second instance = (%v, %v), want (true, nil)", approved, err)
	}
}

func TestRequestFingerprintDetectsParameterTampering(t *testing.T) {
	base := testRequest("ref-1")
	base.Parameters = map[string]string{"pid": "1234"}

	tampered := base
	tampered.Parameters = map[string]string{"pid": "9999"}

	if RequestFingerprint(base) == RequestFingerprint(tampered) {
		t.Fatal("fingerprints match despite different parameters")
	}
}

func TestRequestFingerprintStableAcrossParameterOrder(t *testing.T) {
	a := testRequest("ref-1")
	a.Parameters = map[string]string{"a": "1", "b": "2"}
	b := testRequest("ref-1")
	b.Parameters = map[string]string{"b": "2", "a": "1"}

	if RequestFingerprint(a) != RequestFingerprint(b) {
		t.Fatal("fingerprint depends on map iteration order")
	}
}

func TestRequestFingerprintIgnoresReasonAndRequester(t *testing.T) {
	a := testRequest("ref-1")
	b := testRequest("ref-1")
	b.Reason = "a completely different justification"
	b.Requester = "someone-else@example.com"

	if RequestFingerprint(a) != RequestFingerprint(b) {
		t.Fatal("fingerprint changed when only Reason/Requester differed")
	}
}
