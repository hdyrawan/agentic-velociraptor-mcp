package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"sort"
	"strconv"
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
// Every field is encoded length-prefixed (name:length:bytes;) rather
// than delimiter-separated, so a field value containing what looks like
// a delimiter or a "name:" prefix can never masquerade as a different
// field boundary: two Requests hash equal if and only if every hashed
// field is byte-for-byte equal. (The previous newline-delimited encoding
// allowed a parameter value containing "\nparam:k=v" to collide with a
// genuinely separate parameter — see the collision regression tests in
// hash_test.go.)
//
// Reason and Requester are deliberately excluded: they are investigative
// audit context, not a description of what Velociraptor operation would
// actually run, so minor wording differences there must not cause a
// spurious fingerprint mismatch.
func RequestFingerprint(req Request) string {
	h := sha256.New()

	writeField(h, "op", string(req.Operation))
	writeField(h, "case", req.CaseID)
	writeField(h, "client", req.ClientID)
	writeField(h, "artifact", req.Artifact)
	writeField(h, "profile", req.Profile)
	writeField(h, "hunt", req.HuntID)
	writeField(h, "flow", req.FlowID)
	writeField(h, "upload", req.UploadName)
	writeField(h, "label", req.Label)
	writeField(h, "all", strconv.FormatBool(req.TargetAll))

	keys := make([]string, 0, len(req.Parameters))
	for k := range req.Parameters {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	writeField(h, "paramcount", strconv.Itoa(len(keys)))
	for _, k := range keys {
		writeField(h, "paramkey", k)
		writeField(h, "paramvalue", req.Parameters[k])
	}

	clientIDs := append([]string(nil), req.ClientIDs...)
	sort.Strings(clientIDs)
	writeField(h, "clientidcount", strconv.Itoa(len(clientIDs)))
	for _, id := range clientIDs {
		writeField(h, "clientid", id)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// writeField encodes one named field into the fingerprint hash as
// name:length:bytes; — the explicit length prefix is what makes the
// encoding injective for arbitrary value bytes (including newlines,
// colons, and semicolons). Field names are fixed compile-time constants
// containing no ':' so the prefix itself is unambiguous.
func writeField(h hash.Hash, name, value string) {
	fmt.Fprintf(h, "%s:%d:", name, len(value))
	io.WriteString(h, value)
	io.WriteString(h, ";")
}
