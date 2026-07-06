// Package response contains a small, MCP-tool-agnostic response
// envelope embedded by tool response types so that "did this succeed,
// come back empty, not find anything, or fail" is a predictable,
// documented field (status) instead of an ad-hoc combination of Mode and
// Message strings that differed per tool.
//
// This does not replace any tool's existing domain-specific fields
// (Mode, Client, Artifacts, ...) or velo_health_check's pre-existing
// "ok"/"error" Status field, which predates this package and is left
// untouched to avoid a breaking wire change; see
// docs/security-model.md's "Evidence honesty" section.
package response

// Status is the normalized status vocabulary embedded in v0.2.0+ tool
// response envelopes.
type Status string

const (
	StatusSuccess  Status = "success"
	StatusEmpty    Status = "empty"
	StatusNotFound Status = "not_found"
	StatusError    Status = "error"
)

// Result is a minimal reusable status/message block. Tool handlers with
// richer response shapes can embed this or map it onto their public
// schema without changing the status vocabulary.
type Result struct {
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
}

func Success(message string) Result {
	return Result{Status: StatusSuccess, Message: message}
}

func Empty(message string) Result {
	return Result{Status: StatusEmpty, Message: message}
}

func NotFound(message string) Result {
	return Result{Status: StatusNotFound, Message: message}
}

func Error(message string) Result {
	return Result{Status: StatusError, Message: message}
}

// StatusForCount returns success when at least one row/item was returned
// and empty otherwise. It intentionally does not treat empty as an error:
// a bounded read that finds no data is usually a valid investigation
// observation.
func StatusForCount(count int) Status {
	if count == 0 {
		return StatusEmpty
	}
	return StatusSuccess
}
