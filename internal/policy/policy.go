// Package policy implements the MCP-layer policy engine: a
// defense-in-depth control that sits on top of Velociraptor-native ACLs
// and decides, for a given tool call, whether it is allowed outright,
// allowed only with approval, or blocked.
//
// This package must never be the only thing standing between an agent
// and a dangerous Velociraptor operation. The Velociraptor-side API
// identity permissions (see docs/velociraptor-permissions.md) are the
// primary boundary; this package narrows further and makes the
// narrowing auditable.
package policy

import "github.com/hdyrawan/agentic-velociraptor-mcp/internal/config"

// Engine evaluates policy decisions from a loaded config.PolicyConfig.
//
// TODO(v0.1.0+): implement Engine methods (e.g. EvaluateArtifactCollect,
// EvaluateHuntStart, EvaluateUploadDownload) once real tool handlers
// exist in internal/mcpserver. For the v0.0.x skeleton only the type and
// constructor are provided.
type Engine struct {
	cfg config.PolicyConfig
}

// NewEngine builds an Engine from policy configuration.
func NewEngine(cfg config.PolicyConfig) *Engine {
	return &Engine{cfg: cfg}
}

// ReadOnly reports whether the engine is operating in read-only mode,
// in which case every write-capable tool must be refused before any
// approval or allowlist check is even consulted.
func (e *Engine) ReadOnly() bool {
	return e.cfg.Mode == config.PolicyModeReadOnly
}

// RawVQLAllowed always returns false in the current codebase: no
// released version exposes raw VQL execution, regardless of config.
// This method exists so callers have one place to check rather than
// reading e.cfg.AllowRawVQL directly, and so the "always false" decision
// is visible and greppable.
func (e *Engine) RawVQLAllowed() bool {
	return false
}

// ArtifactAllowed reports whether name is present in the artifact
// allowlist.
func (e *Engine) ArtifactAllowed(name string) bool {
	for _, a := range e.cfg.AllowedArtifacts {
		if a == name {
			return true
		}
	}
	return false
}

// AllowListAllArtifacts reports whether velo_list_artifact_names and
// velo_get_artifact_details may serve artifacts outside the configured
// allowlist. This only affects visibility (listing/reading metadata),
// never collection: ArtifactAllowed remains the sole gate for actually
// using an artifact in a collection or hunt.
func (e *Engine) AllowListAllArtifacts() bool {
	return e.cfg.AllowListAllArtifacts
}

// ProfileAllowed reports whether name is present in the DFIR profile
// allowlist.
func (e *Engine) ProfileAllowed(name string) bool {
	for _, p := range e.cfg.AllowedProfiles {
		if p == name {
			return true
		}
	}
	return false
}

// RequiresApproval reports whether the named operation category must go
// through the approval workflow before it can run.
//
// DEPRECATED (v0.8.0+): as of v0.8.0, no handler in internal/mcpserver
// consults this method. Every approval-gated tool calls verifyApproval
// unconditionally, which means removing an entry from
// policy.require_approval_for in the config has NO effect on whether
// approval is required for that operation. The field is retained as
// documentation of intent and for any future handler that wants
// per-operation opt-out. Do NOT rely on it to disable approval for any
// operation — approval is enforced by the tool handlers, not by this
// list. If you want to disable a write-capable tool entirely, use
// policy.mode: read_only (which the handlers DO consult) instead.
func (e *Engine) RequiresApproval(operation string) bool {
	for _, op := range e.cfg.RequireApprovalFor {
		if op == operation {
			return true
		}
	}
	return false
}

// MaxHuntClients returns the configured cap on how many clients a single
// hunt may target.
func (e *Engine) MaxHuntClients() int {
	return e.cfg.MaxHuntClients
}

// TargetAllAllowed reports whether "all clients" scoping is permitted
// for hunts/collections.
func (e *Engine) TargetAllAllowed() bool {
	return e.cfg.AllowTargetAll
}
