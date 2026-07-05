package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// RequestFingerprint deterministically hashes the security-relevant
// fields of a Request. It is used to detect whether a Request being
// executed still matches the Request that was actually approved (i.e.
// an approved "collect Generic.Client.Info from C.abc" decision must not
// be reusable to collect a different artifact or a different client).
//
// TODO(v0.2.0): call this from the Store implementation and from tool
// handlers at execution time, and refuse to proceed on mismatch.
func RequestFingerprint(req Request) string {
	h := sha256.New()
	fmt.Fprintf(h, "op=%s\ncase=%s\nclient=%s\nartifact=%s\nprofile=%s\nhunt=%s\nflow=%s\n",
		req.Operation, req.CaseID, req.ClientID, req.Artifact, req.Profile, req.HuntID, req.FlowID)
	return hex.EncodeToString(h.Sum(nil))
}
