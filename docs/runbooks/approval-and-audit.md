# Runbook: approvals and audit in operation

Audience: the operator(s) running a controlled pilot. Companion to
[approval-flow.md](../approval-flow.md) (the workflow's design) — this
runbook is the day-to-day procedure. All paths below are placeholders;
substitute your deployment's real ones.

## Creating an approval (human operator, never the MCP client)

Approvals are created and decided **only** by the `approve` CLI
subcommand, run by a human against the same store file the server's
`approval.store_path` points at. No MCP tool can create or decide one —
an agent can only *reference* an approval that a human already made.

```sh
agentic-velociraptor-mcp approve \
  --store /path/to/approvals.json \
  --reference CASE-1234-01 \
  --operation collect_artifact \
  --case-id CASE-1234 \
  --reason "triage requested by IR lead" \
  --requester analyst@example.com \
  --client-id C.1234abcd5678ef90 \
  --artifact Generic.Client.Info \
  --approved-by ir-lead@example.com
```

Operational rules:

- **Approve the exact request, not a category.** The stored request is
  fingerprinted over operation, case ID, client ID, artifact/profile,
  parameters, and hunt scope. A tool call that differs in any of those
  is rejected even with a valid reference — so get the target details
  from the requester *before* running `approve`, and copy them exactly.
- **`--requester` and `--approved-by` should be different people.**
  The CLI accepts the same identity for both and does not enforce the
  separation yet — enforce it procedurally: the person who asked must
  not be the person who runs `approve`.
- **One approval, one execution.** The approval is consumed before the
  Velociraptor call is made; even a failed call burns it (deliberate —
  re-approval is cheaper than replay risk). Exceptions where the
  approval is *preserved*: tool-level precondition failures detected
  before consumption (unsupported backend capability, unsupported
  `client_ids` hunt scope in real mode, unsupported IOC kind).
- **Approvals expire.** `approval.ttl_seconds` (measured from creation)
  bounds how long an approved-but-unused reference stays usable.
  Expired means create a new one; there is no extension command.
- To record a denial (visible, auditable "no"): add `--deny` and
  optionally `--note "reason"`.

## Approval store hygiene

- The store file and its directory belong to the service account; keep
  them out of any shared/world-readable location and out of version
  control (it contains case IDs, targets, and reasons — investigation
  metadata, not secrets, but still sensitive).
- The MCP server only ever consumes; the CLI only ever creates/decides.
  If the file is deleted or corrupted, every gated tool degrades to
  `not_found` responses — fail closed, no write can happen.
- `FileStore` uses OS-level file locking for cross-process safety, but
  the pilot posture is still single-operator: don't script concurrent
  bulk approvals.

## Audit log operation

- **Location and permissions**: `audit.path` (JSONL) is written `0600`
  by the service account. The containing directory must exist and be
  writable by that account only. Treat it as an investigation record:
  retention, backup, and access control per your organization's
  evidence-handling requirements.
- **Rotation**: set `audit.max_size_bytes` (rotate threshold) and
  `audit.max_files` (rotated copies retained as `<path>.1`, `.2`, ...).
  The shipped examples use 100 MiB × 10 files. Zero `max_size_bytes`
  disables rotation — acceptable only for local development.
- **Shipping**: if you forward audit logs centrally, ship the rotated
  files; don't point two writers at the live file.
- **Redaction**: `audit.redact_fields` is additive to a hard-coded
  redaction list (`internal/audit/sanitize.go`), which recurses into
  nested structures. Never remove the shipped entries from the example
  list; add your own deployment-specific field names on top.

## Audit failure is fail-closed for writes — expect it, don't "fix" it by disabling audit

If the audit sink cannot record a write-capable operation, the
operation is **blocked before the approval is consumed and before any
Velociraptor call** (see `gateAuditForWrite`; regression-tested in
`internal/mcpserver`). Symptoms: gated tools suddenly returning an
audit-related error while read-only tools still work.

Response procedure:

1. Check disk space and permissions on `audit.path`'s directory.
2. Check rotation state (a rename can fail if a rotated file is held
   open by a log shipper).
3. Restore audit writability, then re-request approval for the blocked
   operation (the original approval was preserved only if the failure
   happened at the audit gate; verify with `approve`-side inspection
   before assuming).
4. Never set `audit.enabled: false` to unblock writes. If you cannot
   restore audit storage promptly, switch `policy.mode` to `read_only`
   instead (see [rollback.md](rollback.md)) — visibility keeps working
   and nothing mutates unaudited.

## Reviewing the audit log

Every tool call yields exactly one event with outcome `success`,
`blocked`, or `error`. A useful daily pilot review:

```sh
# All blocked/errored write attempts in today's log
grep -E '"outcome":"(blocked|error)"' /path/to/audit.jsonl | tail -50

# Everything a specific approval reference was used for
grep '"approval_id":"CASE-1234-01"' /path/to/audit.jsonl
```

Investigate every `blocked` event you did not expect: it is either an
agent probing beyond its allowlist (worth knowing) or a configuration
gap (worth fixing).
