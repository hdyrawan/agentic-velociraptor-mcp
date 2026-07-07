# Pre-RC security review checklist (v0.10.4)

Complete every item before switching any deployment to the controlled
pilot (v1.0.0-rc.1). Each check names the command or artifact that
proves it — check the deployed build, not just the repository. Archive
the completed checklist with the pilot's records.

Repository-level checks assume the released tag checked out
(`git checkout v0.10.4`) and `go build`/`go test` available.

## 1. Tool inventory

- [ ] `go test ./internal/mcpserver/ -run 'TestNewRegistersExactlyTwentyEightTools|TestNewNeverRegistersUnsafeTools' -v`
      passes (exact-inventory and no-unsafe-tools tests in
      `server_test.go`).
- [ ] Against the **deployed** artifact: MCP Inspector `tools/list`
      returns exactly 28 tools
      ([mcp-client-integration.md](mcp-client-integration.md) smoke
      checklist).
- [ ] No tool named or shaped like raw VQL/generic query execution;
      no tool input accepts a query string:
      `grep -rn "run_vql\|RunVQL" internal/mcpserver/` returns no
      registered tool.

## 2. Approval gates

- [ ] Every write-capable tool is suffixed `_with_approval` and routes
      through `verifyApproval`/`consumeApproval`
      (`internal/mcpserver/server.go`); no tool calls
      `Store.Create`/`Store.Decide`.
- [ ] `go test ./internal/mcpserver/ ./internal/approval/` passes
      (fingerprint match, one-shot consumption, expiry, denial,
      approval-preserved-on-precondition-failure regression tests).
- [ ] Live: a gated call with an unknown reference returns
      `status: "not_found"` and executes nothing; a mismatched-target
      call against a valid approval returns `status: "error"` without
      consuming it.
- [ ] Procedural control in place: requester ≠ approver for every
      approval (the CLI does not enforce this — see
      [runbooks/approval-and-audit.md](runbooks/approval-and-audit.md)).

## 3. Audit fail-closed

- [ ] `audit.enabled: true` and `audit.path` set in the deployed
      config; rotation configured (`max_size_bytes` > 0, `max_files`).
- [ ] Audit file and directory owned by the service account, file mode
      `0600`.
- [ ] `go test ./internal/mcpserver/ -run TestAuditWriteFailure` passes
      (audit-sink failure blocks the write before approval consumption
      and before any Velociraptor call).
- [ ] Redaction list in config is the shipped set or a superset.

## 4. Artifact/profile allowlists

- [ ] `policy.allowed_artifacts` and `policy.allowed_profiles` are
      exact names, reviewed and signed off; no entry you cannot explain.
- [ ] Every allowlisted artifact confirmed present in the deployed
      Velociraptor server's catalog (`velo_get_artifact_details`).
- [ ] `Generic.Detection.HashHunter` present **iff** the pilot uses
      hash IOC hunts.
- [ ] `allow_list_all_artifacts: false`, `allow_target_all: false`,
      `allow_raw_vql: false` (the last is also enforced by
      `config.Validate`).
- [ ] `go test ./internal/config/` passes (includes the shipped-example
      safety tests).

## 5. IOC support limits (honesty check)

- [ ] Team briefed: only `kind: "hash"` performs a real hunt
      (`Generic.Detection.HashHunter`); `ip`/`domain`/`process`/`path`
      fail closed **before** approval lookup with a clear error
      ([tool-reference.md](tool-reference.md) "IOC kind support
      status").
- [ ] Live: an `ip`-kind call returns the unsupported-kind error and
      leaves any referenced approval unconsumed.

## 6. Named-source results

- [ ] Live: `velo_get_flow_results` against a multi-source artifact
      (e.g. `Generic.Client.Info`) with no `source` returns
      `status: "source_required"` with real `available_sources`; retry
      with a listed `source` returns rows; an invalid `source` is
      rejected.

## 7. Upload/download limits

- [ ] `velociraptor.max_rows`, `max_result_bytes`, `max_upload_bytes`
      set to reviewed values (shipped examples: 500 / 1 MiB / 50 MiB).
- [ ] `download_dir` empty unless evidence download is deliberately in
      scope for the pilot; if set, directory pre-created, service-
      account-owned, and its retention documented.
- [ ] `max_hunt_clients` set to the pilot's reviewed cap (examples: 25).

## 8. TLS / connection config

- [ ] `api.config.yaml` files verify the server certificate against
      the pinned server name (`pinned_server_name`, or Velociraptor's
      default `VelociraptorServer`); server-name verification has NOT
      been disabled or loosened to work around a mismatch
      ([velociraptor-permissions.md](velociraptor-permissions.md)).
- [ ] Reader and investigator identities are distinct, least-privilege,
      and hold nothing from the "never grant" table.

## 9. Config and secret file permissions

- [ ] Both `api.config.yaml` files are mode `0600` (enforced at load —
      verify anyway: `stat -c '%a %U' /path/to/*.api.config.yaml`).
- [ ] Config YAML, approval store, and audit directory are service-
      account-owned and not world/group-writable.
- [ ] Secrets mounted read-only into any container; none baked into
      the image, none in environment variables.

## 10. Secret scanning

- [ ] Repository/deploy artifacts scanned — this must return nothing
      but placeholders and documentation:

      ```sh
      grep -RInE "BEGIN (RSA |EC )?(CERTIFICATE|PRIVATE KEY)" \
        examples docs README.md PROJECT_STATE.md PROJECT_PLAN.md
      grep -RInE "(api_key|password|secret|token):\s*[^<\"' ]" \
        examples docs | grep -v redact
      ```

- [ ] No real Velociraptor config, client config, base64 blob, or
      machine-specific path committed anywhere
      (`git log --stat` review of the deploy diff).

## Sign-off

| Role | Name | Date |
|---|---|---|
| Operator | | |
| Approver (DFIR lead) | | |
| Velociraptor server owner | | |
