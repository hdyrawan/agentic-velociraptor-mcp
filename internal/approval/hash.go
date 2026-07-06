package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// RequestFingerprint deterministically hashes the security-relevant
// targeting fields of a Request: Operation, CaseID, ClientID, Artifact,
// Parameters, Profile, HuntID, FlowID, UploadName, and the hunt scope
// fields ClientIDs/Label/TargetAll. It is used to detect whether a
// Request being executed still matches the Request that was actually
// approved (i.e. an approved "collect Generic.Client.Info from C.abc"
// decision must not be reusable to collect a different artifact, a
// different client, or with different parameters; an approved "hunt
// label=windows" decision must not be reusable against "all clients" or
// a different artifact/profile).
//
// Reason and Requester are deliberately excluded: they are investigative
// audit context, not a description of what Velociraptor operation would
// actually run, so minor wording differences there must not cause a
// spurious fingerprint mismatch.
func RequestFingerprint(req Request) string {
	h := sha256.New()
	fmt.Fprintf(h, "op=%s\ncase=%s\nclient=%s\nartifact=%s\nprofile=%s\nhunt=%s\nflow=%s\nupload=%s\nlabel=%s\nall=%t\n",
		req.Operation, req.CaseID, req.ClientID, req.Artifact, req.Profile, req.HuntID, req.FlowID, req.UploadName, req.Label, req.TargetAll)

	keys := make([]string, 0, len(req.Parameters))
	for k := range req.Parameters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(h, "param:%s=%s\n", k, req.Parameters[k])
	}

	clientIDs := append([]string(nil), req.ClientIDs...)
	sort.Strings(clientIDs)
	for _, id := range clientIDs {
		fmt.Fprintf(h, "clientids:%s\n", id)
	}

	return hex.EncodeToString(h.Sum(nil))
}
