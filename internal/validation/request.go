package validation

import (
	"fmt"
	"regexp"
)

const (
	caseIDMaxLength     = 128
	reasonMaxLength     = 1024
	requesterMaxLength  = 256
	uploadNameMaxLength = 1024
	maxCollectionParams = 32
	maxParamKeyLength   = 128
	maxParamValueLength = 4096
)

// approvalReferencePattern matches human-chosen approval references
// (e.g. ticket numbers): letters, digits, dot, dash, and underscore.
// This is deliberately permissive on content but strict on shape,
// rejecting anything that could be a VQL fragment or path traversal
// attempt.
var approvalReferencePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// checkNoControlChars rejects NUL bytes and other C0 control characters
// (tabs allowed, newlines rejected) in single-line fields. Identifiers
// and parameter values that participate in approval fingerprinting and
// audit lines have no legitimate need for a newline, and rejecting one
// here is defense in depth on top of approval.RequestFingerprint's
// injective length-prefixed encoding.
func checkNoControlChars(field, s string) error {
	for _, r := range s {
		if r == 0 || (r < 0x20 && r != '\t') {
			return fmt.Errorf("validation: %s contains a control character", field)
		}
	}
	return nil
}

// checkNoControlCharsMultiline is checkNoControlChars for genuinely
// multi-line free text (an operator's justification, an upload's
// VFS-path-shaped name): tabs and newlines allowed, every other C0
// control character rejected.
func checkNoControlCharsMultiline(field, s string) error {
	for _, r := range s {
		if r == 0 || (r < 0x20 && r != '\t' && r != '\n') {
			return fmt.Errorf("validation: %s contains a control character", field)
		}
	}
	return nil
}

// CaseID validates the investigation/case identifier required on every
// approval-gated request.
func CaseID(s string) error {
	if s == "" {
		return fmt.Errorf("validation: case_id is required")
	}
	if len(s) > caseIDMaxLength {
		return fmt.Errorf("validation: case_id exceeds %d characters", caseIDMaxLength)
	}
	return checkNoControlChars("case_id", s)
}

// Reason validates the operator/agent-supplied justification required on
// every approval-gated request.
func Reason(s string) error {
	if s == "" {
		return fmt.Errorf("validation: reason is required")
	}
	if len(s) > reasonMaxLength {
		return fmt.Errorf("validation: reason exceeds %d characters", reasonMaxLength)
	}
	return checkNoControlCharsMultiline("reason", s)
}

// Requester validates the identity of whoever is asking for an
// approval-gated operation.
func Requester(s string) error {
	if s == "" {
		return fmt.Errorf("validation: requester is required")
	}
	if len(s) > requesterMaxLength {
		return fmt.Errorf("validation: requester exceeds %d characters", requesterMaxLength)
	}
	return checkNoControlChars("requester", s)
}

// ApprovalReference validates the syntactic shape of a human-chosen
// approval reference string before it is used as an approval.Store
// lookup key.
func ApprovalReference(s string) error {
	if !approvalReferencePattern.MatchString(s) {
		return fmt.Errorf("validation: invalid approval reference %q", s)
	}
	return nil
}

// UploadName validates a flow upload's component/name identifier. This
// is deliberately a control-character/length check rather than a strict
// charset regexp, since Velociraptor upload names can be arbitrary VFS
// paths; internal/mcpserver never uses this value to construct a local
// filesystem path directly (see tools_flows.go), so path traversal
// sequences are rejected as a defense-in-depth measure, not because they
// are dereferenced here.
func UploadName(s string) error {
	if s == "" {
		return fmt.Errorf("validation: upload name is required")
	}
	if len(s) > uploadNameMaxLength {
		return fmt.Errorf("validation: upload name exceeds %d characters", uploadNameMaxLength)
	}
	if err := checkNoControlCharsMultiline("upload name", s); err != nil {
		return err
	}
	return nil
}

// CollectionParameters validates agent-supplied artifact collection
// parameters before they are bound into an approval.Request or a
// velociraptor.CollectionRequest. Keys and values are bounded in count
// and length and must not contain control characters; this is
// independent of and in addition to internal/vql's safe parameter
// binding.
func CollectionParameters(params map[string]string) error {
	if len(params) > maxCollectionParams {
		return fmt.Errorf("validation: at most %d collection parameters allowed, got %d", maxCollectionParams, len(params))
	}
	for k, v := range params {
		if k == "" || len(k) > maxParamKeyLength {
			return fmt.Errorf("validation: collection parameter name %q is empty or exceeds %d characters", k, maxParamKeyLength)
		}
		if len(v) > maxParamValueLength {
			return fmt.Errorf("validation: collection parameter %q value exceeds %d characters", k, maxParamValueLength)
		}
		if err := checkNoControlChars(fmt.Sprintf("collection parameter %q", k), k+v); err != nil {
			return err
		}
	}
	return nil
}
