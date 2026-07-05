# Approval flow

Status: draft design; `internal/approval` currently only defines types
(`Request`, `Decision`, the `Store` interface). No approval mechanism is
implemented yet — see PROJECT_PLAN.md v0.2.0.

## Operations that require approval

Configured via `policy.require_approval_for` (see
[configuration.md](configuration.md)); the stable default set is:

- `collect_artifact`
- `collect_dfir_profile`
- `start_hunt`
- `start_dfir_hunt`
- `cancel_flow`
- `cancel_hunt`
- `download_flow_upload`

Every tool suffixed `_with_approval` maps to one of these categories.

## Request shape

An `approval.Request` (see `internal/approval/approval.go`) always
carries:

- `operation` — one of the categories above.
- `case_id` — required; ties the request to an investigation.
- `reason` — required; operator/agent-supplied justification.
- The concrete target: `client_id`, `artifact`, `profile`, `hunt_id`, or
  `flow_id`, whichever apply.

A request with an empty `case_id` or `reason` must be rejected by the
store, not silently accepted with empty values.

## Decision shape

An `approval.Decision` records `approved` (bool), `approved_by`, a
timestamp, and an optional note. `approved_by` must be attributable to a
human or system distinct from the requesting MCP session — a tool
handler must never be able to construct a `Decision` on behalf of its
own request.

## Fingerprinting

`approval.RequestFingerprint` hashes the security-relevant fields of a
`Request` (operation, case ID, client/artifact/profile/hunt/flow). Before
executing a write-capable operation, a tool handler must recompute the
fingerprint of the operation it is about to perform and confirm it
matches the fingerprint of the approved `Request` — not just that *some*
approval exists. This prevents an approved "collect Generic.Client.Info
from C.abc" decision from being reused to justify collecting a different
artifact or targeting a different client.

## Open design questions (to resolve before v0.2.0)

1. **Approval transport.** Candidates: an operator-facing CLI/TUI
   prompt in the same process, a ticketing system integration (e.g. a
   webhook to Slack/Jira with an approve/deny link), or a separate small
   approval service. Whatever is chosen must make it structurally hard
   for the requesting agent session to also be the approver.
2. **Approval lifetime and single-use.** Approvals should expire quickly
   (minutes, not hours) and be consumed on first successful use, so a
   leaked or replayed approval can't authorize repeated operations.
3. **Store persistence.** In-memory is acceptable for a single-process
   deployment; anything multi-instance needs a shared store.
4. **Audit linkage.** Every audit `Event` for an approval-gated tool must
   carry the `approval_id` (see `internal/audit/audit.go`), so the audit
   log alone (without querying the approval store) shows that a decision
   was consulted.

## Non-goals

- Auto-approval based on heuristics or agent self-attestation. Approval
  means a human (or an explicitly designated external system) decided,
  not that the agent asserted its own request was safe.
