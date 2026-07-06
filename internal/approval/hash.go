package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// RequestFingerprint deterministically hashes the security-relevant
// targeting fields of a Request: Operation, CaseID, ClientID, Artifact,
// Parameters, Profile, HuntID, FlowID, and UploadName. It is used to
// detect whether a Request being executed still matches the Request
// that was actually approved (i.e. an approved "collect
// Generic.Client.Info from C.abc" decision must not be reusable to
// collect a different artifact, a different client, or with different
// parameters).
//
// Reason and Requester are deliberately excluded: they are investigative
// audit context, not a description of what Velociraptor operation would
// actually run, so minor wording differences there must not cause a
// spurious fingerprint mismatch.
func RequestFingerprint(req Request) string {
	h := sha256.New()
	fmt.Fprintf(h, "op=%s\ncase=%s\nclient=%s\nartifact=%s\nprofile=%s\nhunt=%s\nflow=%s\nupload=%s\n",
		req.Operation, req.CaseID, req.ClientID, req.Artifact, req.Profile, req.HuntID, req.FlowID, req.UploadName)

	keys := make([]string, 0, len(req.Parameters))
	for k := range req.Parameters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(h, "param:%s=%s\n", k, req.Parameters[k])
	}

	return hex.EncodeToString(h.Sum(nil))
}
