# Runbook: containment and rollback

Audience: the operator(s) running a controlled pilot. Procedures for
containing this MCP server quickly and rolling it back to a known-good
state. Ordered from least to most disruptive — you can stop at any step
once contained. All paths are placeholders.

## Decide what you're containing

- **Suspicious agent behavior** (unexpected `blocked` events, probing
  outside the allowlist, unexplained approvals requested): step 1 is
  usually enough while you investigate.
- **Suspected server bug or bad release**: steps 1 + 4.
- **Suspected credential compromise** (either `api.config.yaml`
  identity): go straight to step 3 — Velociraptor-side revocation is
  the real control; nothing in this server substitutes for it.

## Step 1: emergency switch to read-only

Edit the deployed `config.yaml`:

```yaml
policy:
  mode: read_only
```

then restart the server process (the MCP client host relaunches it, or
re-run the `docker run`/binary invocation). Config is read at startup
only. Effects: every approval-gated tool refuses to run and audits
itself `blocked`; the 14 read-only tools keep working, so visibility is
retained during the investigation. Existing approvals in the store stay
untouched and unconsumable until mode is restored.

Equivalently (slightly stronger): set `approval.store_path: ""` too —
either switch alone disables the write pilot; both together make the
posture unambiguous to the next person reading the config.

## Step 2: disable MCP client access

The server is a stdio child process of whatever MCP client host runs
it. To cut agent access entirely:

- Remove/disable the server entry in the MCP client's config (Claude
  Desktop `claude_desktop_config.json`, Claude Code `claude mcp remove
  velociraptor`, or the equivalent for your host), then restart that
  client, **or**
- Stop the client host itself, **or**
- Make the launch command fail closed: move the config file aside — a
  set-but-unloadable `--config` makes the server exit at startup, it
  never falls back to a permissive default.

No listening port exists to firewall; stdio-only transport means "no
launcher, no access."

## Step 3: revoke the Velociraptor service identities

This is the authoritative control and lives in Velociraptor, not here —
this server has no independent revocation mechanism.

1. In Velociraptor (GUI user management or `velociraptor` CLI on the
   server), disable or delete the API client users behind
   `read_api_config_path` and `write_api_config_path` (revoke the write
   identity first if you must choose).
2. Delete the corresponding `api.config.yaml` files from the host
   running this server.
3. If compromise is suspected, treat it as a credential incident per
   your organization's process: rotate, review Velociraptor server
   audit logs for use of those identities outside this server's own
   audit trail, and only re-issue identities with the least-privilege
   sets in [velociraptor-permissions.md](../velociraptor-permissions.md).

After revocation, a running server fails its Velociraptor calls with
structured errors (no crash, no fallback to mock mode for a set-but-
invalid path at next restart — it refuses to start, which is the
intended fail-closed behavior).

## Step 4: roll back to a previous image/tag

Deployments should always run a **pinned tag or digest**, never
`:latest` (see [production-deployment.md](../production-deployment.md)),
which makes rollback a redeploy of the previous pin:

```sh
# Container: relaunch the MCP client's server entry against the prior pin
docker run --rm -i ... agentic-velociraptor-mcp:v0.10.2
# or, stronger, by digest:
docker run --rm -i ... agentic-velociraptor-mcp@sha256:<previous-digest>

# From source: rebuild the previous release tag
git checkout v0.10.2
go build -o bin/agentic-velociraptor-mcp ./cmd/agentic-velociraptor-mcp
```

Compatibility notes for rolling back across versions:

- The YAML config schema has been additive; a newer config generally
  loads under an older binary, but re-validate: run the old binary with
  `--config` and confirm it starts and `velo_health_check` succeeds.
- The approval store: entries created by a newer version are readable
  by older versions within the v0.x line, but the fingerprint algorithm
  changed in v0.8.0 — do not roll back across that boundary with
  pending approvals; re-create them instead.
- Audit logs are append-only JSONL; nothing to migrate.

## Post-containment checklist

- [ ] Audit log preserved (copy the live file and rotated copies before
      any cleanup).
- [ ] Approval store reviewed: every consumed approval maps to an
      expected, explained operation.
- [ ] Velociraptor server-side logs cross-checked against this server's
      audit trail for the incident window.
- [ ] Root cause written up before restoring `controlled` mode.
- [ ] Restore path: re-enable in the reverse order (identities →
      client access → `controlled` mode), re-running the
      [security-review-checklist-v0.10.4.md](../security-review-checklist-v0.10.4.md)
      gate first.
