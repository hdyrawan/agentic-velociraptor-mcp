# DFIR profiles

Status: **46 profile definitions** exist under `profiles/` (the original
15 from v0.4.0–v0.7.0 plus **31 curated, catalog-verified profiles added
in v0.10.0**). All are definition-only profile catalog entries. Adding
these YAML files does not add any MCP tool, collection, hunt, download,
raw VQL, shell, or endpoint-changing logic — the callable MCP tool
inventory is unchanged (still 28 tools). Some profiles require specific
approval for high-sensitivity/privacy cases (for example credential
theft, browser activity, shell history, user activity, SSH trust, and
timeline reconstruction). See PROJECT_PLAN.md v0.4.0.

## The curated artifact catalog (v0.10.0)

Every artifact any profile references must be a reviewed entry in
`catalog/artifacts.yaml` (loaded by `internal/dfir.LoadCatalog`). The
catalog is an **authoring-time and test-time control, not a runtime
permission path**:

- It does **not** grant collection. Whether an artifact may actually run
  is still gated exclusively by `policy.allowed_artifacts` at runtime
  (`internal/dfir/validate.go` → `policy.Engine.ArtifactAllowed`). Adding
  a catalog entry never widens or bypasses that allowlist, and there is
  no wildcard/prefix matching — only exact, reviewed names.
- Its job is to guarantee every profile artifact is a known, reviewed,
  exact name carrying recorded metadata. Each entry records: `name`,
  `target_os`, `category`, `risk_level`, `requires_approval`,
  `sensitivity`, `verified`, and `notes`.
- `internal/dfir.ValidateProfileAgainstCatalog` plus the tests
  `TestEveryShippedProfileArtifactIsInCatalog` and
  `TestProfileApprovalConsistentWithCatalog` (in
  `internal/dfir/catalog_test.go`) enforce, at `go test` time, that no
  profile references an artifact absent from the catalog, and that any
  profile bundling a `requires_approval: true` artifact is itself
  `requires_approval: true` — a low-friction profile can never smuggle a
  sensitive artifact past the approval gate.

### Verified vs candidate artifacts

`verified: true` catalog entries were confirmed present in a real
Velociraptor server's artifact catalog (a lab running
`wlambert/velociraptor`, a 433-artifact catalog, via
`velociraptor artifacts list`, 2026-07-07). **All 31 v0.10.0 profiles
reference only `verified: true` artifacts.**

`verified: false` entries are the pre-existing **illustrative** names
carried by the original v0.4.0–v0.7.0 profiles (`System.Hash.Hunt`,
`Windows.Browser.History`, `Windows.System.EventLogs`,
`Windows.System.Persistence`, `Linux.System.Pslist`, etc.). They are
retained so the older profiles still validate, and each catalog entry's
`notes` names the nearest real, verified artifact an operator should
substitute after confirming against their own deployment's catalog. New
profiles were deliberately **not** built on these unverified names, to
avoid shipping misleading production-ready coverage.

## What a profile is

A DFIR profile (`internal/dfir.Profile`) is a named, reviewed bundle of
Velociraptor artifacts corresponding to one investigation intent, e.g.
"windows_basic_triage" = client info + process list + netstat. A profile
grants no permission beyond what its constituent artifacts already have
in `policy.allowed_artifacts`; see `internal/dfir/validate.go`. Profiles
exist so an agent (and the human approving its requests) reasons in
investigation terms ("run ransomware triage against C.abc123") instead
of naming individual artifacts or, worse, writing VQL.

## Investigation cases this design must support

| Case | Supporting profile(s) |
|------|------------------------|
| Endpoint initial triage | `windows_basic_triage`, `windows_system_inventory`, `linux_basic_triage`, `linux_system_inventory`, `cross_platform_identity` |
| Process investigation | `windows_process_network_triage`, `linux_process_analysis`, `cross_platform_process` |
| Network connection review | `windows_network_connections`, `linux_network_connections`, `cross_platform_network` |
| Persistence investigation | `windows_persistence_triage`, `windows_scheduled_task_persistence`, `windows_wmi_persistence`, `windows_service_persistence`, `linux_persistence_triage`, `linux_cron_persistence`, `linux_systemd_services` |
| Lateral movement investigation | `windows_lateral_movement_triage`, `windows_authentication_events`, `linux_ssh_trust` (approval) |
| Ransomware triage | `windows_ransomware_triage` |
| Credential theft investigation | `windows_credential_theft_triage`, `windows_powershell_activity` (approval) |
| Execution evidence | `windows_execution_evidence` |
| IOC hunting | `ioc_hash_hunt`, `ioc_ip_hunt`, `ioc_domain_hunt`, `cross_platform_ioc_context`, `cross_platform_local_hashes` |
| Browser activity review (approval) | `windows_browser_history`, `windows_browser_downloads`, `windows_browser_extensions`, `windows_browser_cookies`, `windows_browser_cache`, `windows_browser_activity_triage` |
| User activity review (approval) | `windows_user_activity`, `linux_shell_history` (approval) |
| Windows event log collection | `windows_eventlog_triage`, `windows_authentication_events` |
| Linux endpoint triage | `linux_basic_triage`, `linux_system_inventory`, `linux_process_analysis`, `linux_network_connections`, `linux_auth_logs`, `linux_package_inventory`, `linux_container_triage` |
| Software/package inventory | `windows_system_inventory` (Windows), `linux_package_inventory` (Linux) — no verified cross-platform software artifact exists, so this stays OS-specific by design |
| Privilege/access review | `linux_privilege_escalation`, `linux_ssh_trust` (approval) |
| File evidence retrieval (approval) | via `velo_download_flow_upload_with_approval`, not a profile |
| Timeline generation | `windows_timeline_triage`, `windows_filesystem_timeline` (approval) |
| Hunt across endpoints (approval) | any profile via `velo_start_dfir_hunt_with_approval` |
| Ad hoc IOC hunt, no profile (approval) | `velo_hunt_ioc_with_approval` (hash/ip/domain/process/path) |

## Defined profile catalog (46)

All 46 profiles below are definition-only entries; every artifact each
references is a reviewed `catalog/artifacts.yaml` entry (see "The curated
artifact catalog" above). "Requires approval" mirrors
`requires_approval` in the YAML; it is `yes` whenever the profile bundles
any privacy-sensitive or credential-adjacent artifact (browser data,
shell/console history, user-activity records, SSH trust, full-filesystem
timelines). "Artifacts" is the number of artifacts in the bundle.

| Name | Target OS | Category | Risk | Requires approval | Artifacts |
|------|-----------|----------|------|-------------------|-----------|
| `cross_platform_identity` | any | inventory | low | no | 1 |
| `cross_platform_ioc_context` | any | ioc | medium | no | 2 |
| `cross_platform_local_hashes` | any | ioc | medium | no | 1 |
| `cross_platform_network` | any | network | low | no | 2 |
| `cross_platform_process` | any | process | low | no | 1 |
| `ioc_domain_hunt` | any | ioc | low | no | 1 |
| `ioc_hash_hunt` | any | ioc | low | no | 1 |
| `ioc_ip_hunt` | any | ioc | low | no | 1 |
| `linux_auth_logs` | linux | authentication | medium | no | 3 |
| `linux_basic_triage` | linux | triage | low | no | 3 |
| `linux_container_triage` | linux | containers | medium | no | 2 |
| `linux_cron_persistence` | linux | persistence | medium | no | 1 |
| `linux_network_connections` | linux | network | medium | no | 3 |
| `linux_package_inventory` | linux | inventory | low | no | 4 |
| `linux_persistence_triage` | linux | persistence | medium | no | 1 |
| `linux_privilege_escalation` | linux | privilege | medium | no | 2 |
| `linux_process_analysis` | linux | process | medium | no | 3 |
| `linux_process_network_triage` | linux | triage | medium | no | 2 |
| `linux_shell_history` | linux | user-activity | high | yes | 1 |
| `linux_ssh_trust` | linux | persistence | high | yes | 2 |
| `linux_systemd_services` | linux | persistence | medium | no | 1 |
| `linux_system_inventory` | linux | inventory | low | no | 5 |
| `windows_authentication_events` | windows | authentication | medium | no | 3 |
| `windows_basic_triage` | windows | triage | low | no | 3 |
| `windows_browser_activity_triage` | windows | browser-activity | high | yes | 1 |
| `windows_browser_cache` | windows | browser | high | yes | 2 |
| `windows_browser_cookies` | windows | browser | high | yes | 1 |
| `windows_browser_downloads` | windows | browser | high | yes | 2 |
| `windows_browser_extensions` | windows | browser | high | yes | 1 |
| `windows_browser_history` | windows | browser | high | yes | 3 |
| `windows_credential_theft_triage` | windows | credential-theft | high | yes | 1 |
| `windows_eventlog_triage` | windows | eventlog | medium | no | 1 |
| `windows_execution_evidence` | windows | execution | medium | no | 4 |
| `windows_filesystem_timeline` | windows | timeline | high | yes | 2 |
| `windows_lateral_movement_triage` | windows | lateral-movement | high | no | 1 |
| `windows_network_connections` | windows | network | medium | no | 4 |
| `windows_persistence_triage` | windows | persistence | medium | no | 1 |
| `windows_powershell_activity` | windows | powershell | high | yes | 3 |
| `windows_process_network_triage` | windows | triage | medium | no | 2 |
| `windows_ransomware_triage` | windows | ransomware | high | yes | 6 |
| `windows_scheduled_task_persistence` | windows | persistence | medium | no | 2 |
| `windows_service_persistence` | windows | persistence | medium | no | 3 |
| `windows_system_inventory` | windows | inventory | low | no | 6 |
| `windows_timeline_triage` | windows | timeline | high | yes | 1 |
| `windows_user_activity` | windows | user-activity | high | yes | 3 |
| `windows_wmi_persistence` | windows | persistence | medium | no | 1 |

**Approval rationale.** A profile is `requires_approval: true` when it
collects data that is privacy-sensitive, credential-adjacent, or of a
volume/breadth that warrants a human decision before it runs:

- Browser data (`windows_browser_*`, `windows_browser_activity_triage`)
  — history, downloads, extensions, cookies (session tokens), cache —
  reveals user web activity and can expose session material.
- Shell/console history (`linux_shell_history`,
  `windows_powershell_activity` via PSReadline) can contain typed
  secrets.
- User-activity evidence (`windows_user_activity`) is inherently
  per-user and privacy-bearing.
- SSH trust (`linux_ssh_trust`) exposes trust relationships and outbound
  connection history; note that SSH **private keys are deliberately not
  collected** by any profile.
- Full-filesystem timelines (`windows_filesystem_timeline`,
  `windows_timeline_triage`) enumerate every filename on a volume — large
  and broadly privacy-bearing.

Lower-friction inventory/process/network/persistence profiles are
`requires_approval: false`, but every write-capable MCP tool that would
*act on* a profile (`velo_collect_dfir_profile_with_approval`,
`velo_start_dfir_hunt_with_approval`) is itself approval-gated and
policy-gated regardless of the profile's own flag. Profiles never
weaken those gates.

## Authoring a new profile

1. Add `profiles/<name>.yaml` matching `internal/dfir.Profile`'s strict
   YAML shape (`name`, `display_name`, `description`, `target_os`,
   `category`, `risk_level`, `requires_approval`, `max_runtime_seconds`,
   `max_result_rows`, `max_result_bytes`, `artifacts`). Unknown fields are
   rejected so typos such as `max_runtime_sum` fail during profile loading.
2. Ensure every artifact the profile references is a reviewed entry in
   `catalog/artifacts.yaml`. If it is a new name, add a catalog entry
   with full metadata (`target_os`, `category`, `risk_level`,
   `requires_approval`, `sensitivity`, `verified`, `notes`) first.
   `internal/dfir/catalog_test.go` fails the build if any profile
   references an artifact absent from the catalog, or if a profile
   bundling a `requires_approval: true` catalog artifact is not itself
   `requires_approval: true`.
3. Verify every artifact name against the actual artifact catalog of the
   target Velociraptor server/version and set the catalog entry's
   `verified` flag accordingly — do not ship `verified: true` for a name
   you have not confirmed. Do not build new profiles on `verified: false`
   names.
4. Add each artifact to `policy.allowed_artifacts` in the deployment
   config, and the profile name itself to `policy.allowed_profiles`.
   Both are deliberate, reviewed operator actions — the catalog does
   **not** substitute for this runtime allowlist.
5. Run `velo_validate_dfir_profile` before relying on the profile in a
   collection or hunt.

## IOC profiles specifically

`ioc_hash_hunt`, `ioc_ip_hunt`, and `ioc_domain_hunt` are definition-only
profile catalog entries in this commit. They do not implement IOC hunt
logic by themselves, do not add raw VQL, and do not add endpoint-changing
behavior.

**v0.7.0 implemented the IOC hunt tool** (`velo_hunt_ioc_with_approval`),
but it is deliberately independent of this profile catalog and the
`internal/dfir.Registry`: it validates a `hash`/`ip`/`domain`/`process`/
`path` indicator (`internal/validation.ValidateIOC`) and resolves it
through a fixed `internal/vql.TemplateName` → artifact/parameter mapping
(`vql.Bind`) rather than loading a `profiles/*.yaml` file. The 5
supported template names (`ioc_hash_hunt`, `ioc_ip_hunt`,
`ioc_domain_hunt`, `ioc_process_hunt`, `ioc_path_hunt`) intentionally
mirror this catalog's 3 existing IOC profile names for the first three
kinds, but `ioc_process_hunt`/`ioc_path_hunt` have no corresponding
`profiles/*.yaml` entry — none is needed, since the IOC tool never reads
the profile registry. As with the pre-existing IOC profiles' artifact
names, `vql.Bind`'s resolved artifact names (`System.Hash.Hunt`,
`System.IP.Hunt`, `System.Domain.Hunt`, `System.Process.Hunt`,
`System.Path.Hunt`) are illustrative and unverified against a real
Velociraptor artifact catalog — confirm them against your deployment's
actual catalog before adding them to `policy.allowed_artifacts` (the
gate that actually permits the tool to use them). The tool keeps using
fixed, reviewed templates with validated indicator values passed as
bound parameters — never string-concatenated into a query.
