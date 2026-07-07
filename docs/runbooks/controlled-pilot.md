# Runbook: running the controlled pilot

Audience: the team taking this server from "validated in a lab" to a
real, bounded pilot against a production Velociraptor deployment — the
v1.0.0-rc.1 milestone. This is deliberately staged; do not skip stages.

## Ground rules for the whole pilot

- The pilot's blast radius is bounded by config, and only by config you
  reviewed: exact-name `allowed_artifacts`/`allowed_profiles`,
  `max_hunt_clients` kept small, `allow_target_all: false`.
- Every write requires an out-of-band human approval
  ([approval-and-audit.md](approval-and-audit.md)); the person
  approving is not the person (or agent) requesting.
- Anything unexpected → contain first, investigate second
  ([rollback.md](rollback.md)).
- Do not "fix" a blocked operation by widening config mid-incident.
  Widen the allowlist only through the same review that set it.

## Stage 0 — prerequisites (before any production connection)

- [ ] [security-review-checklist-v0.10.4.md](../security-review-checklist-v0.10.4.md)
      fully checked and archived with the pilot's records.
- [ ] Reader and investigator Velociraptor identities provisioned per
      [velociraptor-permissions.md](../velociraptor-permissions.md),
      reviewed by the Velociraptor server owner, neither holding any
      "never grant" permission.
- [ ] `allowed_artifacts`/`allowed_profiles` signed off by the DFIR team
      that will use them; every artifact confirmed present in the
      deployed server's catalog.
- [ ] Deployment hardened per
      [production-deployment.md](../production-deployment.md): pinned
      image, non-root, read-only secrets mounts, writable audit volume.
- [ ] Named humans: at least one approver and one operator, with the
      rollback runbook read by both.

## Stage 1 — read-only against production (recommended ≥ 1 week)

Deploy with `examples/config/config.readonly.example.yaml` as the
template: `policy.mode: read_only`, **no** `write_api_config_path`, no
approval store. Only the reader identity exists on the host.

Exit criteria:

- [ ] `velo_health_check`, client search/info, artifact catalog, flow
      and hunt listing/results all behave against real data.
- [ ] Multi-source results confirmed: `velo_get_flow_results` against a
      `Generic.Client.Info` collection returns
      `status: "source_required"` with real `available_sources`, and a
      retry with `source` set returns rows (the v0.10.3 fix, exercised
      against your data).
- [ ] Audit log rotating and shipping as expected; daily review habit
      established.
- [ ] No unexplained `blocked`/`error` events.

## Stage 2 — controlled writes in a disposable lab first

Before enabling `controlled` against production, replay the write path
end-to-end in a lab matching production topology
([lab-validation-plan.md](../lab-validation-plan.md)): one approved
collection, one approved label-scoped hunt, one approved hash IOC hunt
(`Generic.Detection.HashHunter` must be in `allowed_artifacts`), one
cancel, one denial, one expiry.

## Stage 3 — controlled pilot against production

Switch to `examples/config/config.controlled.example.yaml` as the
template, with the narrowest allowlist your first real case needs.
During the pilot:

- Review the audit log daily; reconcile every consumed approval against
  the case record it cited.
- Keep `download_dir` empty until a case actually requires evidence
  download, then enable it deliberately and disable it after.
- Treat IOC hunting honestly: only `kind: "hash"` works;
  `ip`/`domain`/`process`/`path` fail closed by design (see
  [tool-reference.md](../tool-reference.md)'s "IOC kind support
  status") — don't promise those workflows to the pilot's users.

## Known gaps going into the pilot (unchanged from v0.10.2/v0.10.3)

Plan the pilot's scope around these; they are documented, not hidden:

1. **Label-scoped hunts not yet live-validated** against a client with
   a verified persistent label (lab-tooling limitation in v0.10.2).
   First production label hunt: preview with `velo_preview_hunt_scope`,
   compare the count against the Velociraptor GUI, and start small.
2. **No Windows client exercised live yet.** First Windows collection
   is a validation event: verify results, note anomalies.
3. **Upload/download path not yet exercised against a real
   file-producing collection.** First evidence download follows the
   same care: verify size/sha256 against the GUI's values.
4. **Explicit `client_ids` hunt scope is unsupported in real mode**
   (no typed RPC); use `label` scoping. The tools refuse before
   consuming the approval.
5. **`ip`/`domain`/`process`/`path` IOC kinds unsupported** until a
   curated artifact set is identified for them.

## Exit criteria: pilot → v1.0.0-rc.1 sign-off

- [ ] Gaps 1–3 above each exercised at least once against production or
      a production-identical environment, results recorded.
- [ ] At least one full case worked end-to-end through the server
      (visibility → approved collection → results → report), with the
      audit trail reconciled.
- [ ] At least one containment drill: switch to `read_only`, confirm
      writes refuse, switch back.
- [ ] Zero unexplained audit events over the pilot window.
- [ ] Known open review items re-triaged — fixed or formally accepted
      for the RC. At minimum: the `approve` CLI accepting the same
      identity as requester and approver (enforced procedurally during
      the pilot, per [approval-and-audit.md](approval-and-audit.md)).
