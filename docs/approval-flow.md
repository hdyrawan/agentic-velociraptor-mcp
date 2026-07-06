# Approval flow

Status: implemented as of v0.4.0 (controlled single-client collection
pilot), extended in v0.6.0 (hunt management), and extended again in
v0.7.0 (IOC hunting). `internal/approval` has a real `Store`
implementation (`FileStore`, `internal/approval/filestore.go`), and
every approval-gated tool (v0.4.0: `velo_collect_artifact_with_approval`,
`velo_collect_dfir_profile_with_approval`,
`velo_cancel_flow_with_approval`,
`velo_download_flow_upload_with_approval`; v0.6.0:
`velo_start_hunt_with_approval`,
`velo_start_dfir_hunt_with_approval`,
`velo_cancel_hunt_with_approval`; v0.7.0: `velo_hunt_ioc_with_approval`)
verifies against it before calling Velociraptor, via the same
fingerprint-checked `verifyAndConsumeApproval` path described below —
**not** a separate, weaker check per tool (see "v0.7.0 fix" note under
"Known limitations"). This document describes the operator-facing
workflow those tools depend on.

## The core guarantee

**No MCP tool can create or decide an approval.** `Store.Create` and
`Store.Decide` are only ever called from the
`agentic-velociraptor-mcp approve` CLI subcommand
(`cmd/agentic-velociraptor-mcp/main.go`), which is not part of the MCP
tool surface and is never invoked by the running MCP server. A human
operator runs it directly, against the same on-disk store file
(`approval.store_path`) the MCP server reads. This is what makes
"approval" a real control: an MCP client (including an LLM driving tool
calls) can *request* that a write-capable operation run, by supplying an
`approval_reference`, but it can never make that reference valid itself.

Configured via `policy.require_approval_for` (see
[configuration.md](configuration.md)); the stable default set is:

- `collect_artifact`
- `collect_dfir_profile`
- `start_hunt`
- `start_dfir_hunt`
- `cancel_flow`
- `cancel_hunt`
- `download_flow_upload`
- `hunt_ioc`

Every tool suffixed `_with_approval` maps to one of these categories.

## Two gates before any approval is even consulted

Every approval-gated tool first checks `mcpserver.writePilotEnabled`:
**both** `policy.mode: controlled` and `approval.store_path` must be
configured, or the tool refuses outright (audited `blocked`) without
ever calling `Store.Get`. `velo_download_flow_upload_with_approval` has
a third, independent gate: `velociraptor.download_dir` must also be
configured, since it is the one tool that discloses raw evidence bytes.

## Step 1: request a collection reference (agent/analyst side)

The MCP tool call itself *is* the request — there is no separate
"preview" tool. The caller supplies:

- `case_id` — required; ties the request to an investigation.
- `reason` — required; justification for the operation.
- `requester` — required; identifies who/what is asking, distinct from
  the human who will approve it.
- The concrete target: `client_id` plus `artifact`/`profile`/`flow_id`/
  `upload_name` as applicable, and (for `velo_collect_artifact_with_approval`)
  optional `parameters`.
- `approval_reference` — a string the caller expects a human to have
  already approved out-of-band (e.g. a ticket number agreed on in a
  chat or ticketing system before the agent ever calls the tool).

If no record with that reference exists yet, the tool responds with
`status: "not_found"` — a normal structured result, not a crash — so the
agent/analyst knows to go get it approved.

## Step 2: create and decide the request (human operator side)

A human runs the `approve` subcommand directly, never through the MCP
server:

```sh
agentic-velociraptor-mcp approve \
  --store /var/lib/agentic-velociraptor-mcp/approvals.json \
  --reference CASE-1234-01 \
  --operation collect_artifact \
  --case-id CASE-1234 \
  --reason "triage requested by IR lead" \
  --requester analyst@example.com \
  --client-id C.1234abcd5678ef90 \
  --artifact Windows.System.Pslist \
  --param pid=1234 \
  --approved-by ir-lead@example.com
```

This both creates the `approval.Request` (`Store.Create`) and records an
approving `approval.Decision` (`Store.Decide`) in one step, since only a
human runs this command and both operations require the same identity's
authorization. Add `--deny` (and optionally `--note "..."`) to record a
denial instead — a denied reference resolves but is never usable.

`--store` must point at the exact same file as the running MCP server's
`approval.store_path`. `FileStore` re-reads the file on every call (no
in-memory caching), specifically so this out-of-process write becomes
visible to a long-running MCP server without a restart.

For hunt/IOC-hunt operations (`start_hunt`, `start_dfir_hunt`,
`cancel_hunt`, `hunt_ioc`), use `--artifact`/`--profile` as applicable,
`--hunt-id` for `cancel_hunt`, and the scope flags `--hunt-client-id`
(repeatable, for an explicit client list), `--label`, and `--all` —
these bind into `approval.Request.ClientIDs`/`Label`/`TargetAll`, which
are part of the fingerprint (see "Fingerprinting" below), so the
approved scope must match the call's scope exactly:

```sh
agentic-velociraptor-mcp approve \
  --store /var/lib/agentic-velociraptor-mcp/approvals.json \
  --reference CASE-1234-02 \
  --operation start_hunt \
  --case-id CASE-1234 \
  --reason "sweep for lateral movement" \
  --requester analyst@example.com \
  --artifact Windows.System.Pslist \
  --label windows \
  --approved-by ir-lead@example.com
```

## Step 3: execute (agent side, again)

The agent calls the same tool again with identical `client_id`/
`artifact`/`profile`/`parameters`/`flow_id`/`upload_name`/`case_id` and
the same `approval_reference`. The tool handler:

1. Looks up the reference via `Store.Get`.
2. Recomputes `approval.RequestFingerprint` over the *current* call's
   targeting fields (operation, case_id, client_id, artifact/profile,
   parameters, flow_id, upload_name) and compares it to the fingerprint
   of the stored `Request`. A mismatch — e.g. approved for
   `Windows.System.Pslist` but called for `Generic.Client.Info` — is
   rejected (`status: "error"`), never silently substituted.
3. Confirms `Decision.Approved` is `true`, the approval has not expired
   (`approval.ttl_seconds`, measured from `Request.CreatedAt`), and has
   not already been consumed.
4. Calls `Store.Consume` **before** calling Velociraptor, so a single
   human approval authorizes at most one execution attempt — even if
   that attempt then fails for an unrelated reason (e.g. a transient
   network error), the same reference cannot be retried; a new one must
   be requested and approved.
5. Only then calls the underlying `velociraptor.Client` method.

Every one of these checks is `errors`/`response.Result`-typed and
produces exactly one audit event (`success`/`blocked`/`error`); see
`internal/mcpserver/server.go`'s `verifyAndConsumeApproval`.

## Fingerprinting

`approval.RequestFingerprint` hashes exactly the targeting fields of a
`Request`: operation, case ID, client ID, artifact, parameters (sorted by
key), profile, hunt ID, flow ID, upload name, and (as of v0.7.0) the
hunt-scope fields client IDs (sorted), label, and target-all. `reason`
and `requester` are deliberately excluded — they are investigative
context, not a description of what Velociraptor operation would run, so
wording differences there must never cause a spurious mismatch.

## Known limitations (v0.4.0/v0.6.0/v0.7.0)

- **Single-analyst pilot, not a multi-writer system.** `FileStore`
  re-reads and rewrites the whole file on every call under an in-process
  mutex; it does not use OS-level file locking, so two operators (or the
  `approve` CLI and the MCP server) writing at the exact same instant
  could in principle race. Acceptable for a controlled pilot; revisit
  before wider rollout.
- **No RPC wiring yet.** Even a fully-approved, fully-verified request
  currently fails at the last step (`velociraptor.ErrNotImplemented`)
  against a real (non-mock) write client, since the hand-authored
  `veloapi` proto mirror doesn't wire real `CollectArtifact`/
  `CancelFlow`/upload/hunt RPCs yet. See PROJECT_PLAN.md's v0.8.0 entry.
- **No revocation.** A pending, undecided request cannot currently be
  withdrawn by the requester; it simply expires via `ttl_seconds` if
  never decided.
- **v0.7.0 fix**: as originally merged in v0.6.0, the three hunt-write
  tools did not actually perform the fingerprint check described above —
  a separate `checkHuntApproval` helper only confirmed the reference was
  approved and unconsumed, not that it matched the call's operation/
  artifact/scope. Any valid unconsumed approval could start/cancel a
  different hunt than approved. This was found in code review before
  merging and fixed by routing those tools (and the new
  `velo_hunt_ioc_with_approval`) through `verifyAndConsumeApproval`, the
  same path this document has always described. If you have an
  operational runbook or automation built against v0.6.0's actual
  (unfingerprinted) behavior, it must be updated: an approval now must
  match the exact artifact/profile/scope it will be used for.

## Non-goals

- Auto-approval based on heuristics or agent self-attestation. Approval
  means a human ran the `approve` CLI, not that the agent asserted its
  own request was safe.
- Reusing one approval across multiple executions, multiple clients, or
  different targets — see "Fingerprinting" and `Store.Consume` above.
