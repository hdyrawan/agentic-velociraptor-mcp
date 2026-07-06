# DFIR profiles

Status: 15 of 15 planned profile definitions exist under `profiles/`.
All are definition-only profile catalog entries. Adding these YAML files
does not add collection, hunt, download, raw VQL, shell, or endpoint-changing
logic. Some profiles require specific approval for high-sensitivity cases
(for example credential theft, browser activity, and timeline reconstruction).
See PROJECT_PLAN.md v0.4.0.

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
| Endpoint initial triage | `windows_basic_triage`, `linux_basic_triage` |
| Process investigation | `windows_process_network_triage`, `linux_process_network_triage` |
| Network connection review | `windows_process_network_triage`, `linux_process_network_triage` |
| Persistence investigation | `windows_persistence_triage`, `linux_persistence_triage` |
| Lateral movement investigation | `windows_lateral_movement_triage` |
| Ransomware triage | `windows_ransomware_triage` |
| Credential theft investigation | `windows_credential_theft_triage` |
| IOC hunting | `ioc_hash_hunt`, `ioc_ip_hunt`, `ioc_domain_hunt` |
| Browser/user activity review (approval) | `windows_browser_activity_triage` |
| Windows event log collection | `windows_eventlog_triage` |
| Linux endpoint triage | `linux_basic_triage`, `linux_process_network_triage`, `linux_persistence_triage` |
| File evidence retrieval (approval) | via `velo_download_flow_upload_with_approval`, not a profile |
| Timeline generation | `windows_timeline_triage` |
| Hunt across endpoints (approval) | any profile via `velo_start_dfir_hunt_with_approval` |
| Ad hoc IOC hunt, no profile (approval) | `velo_hunt_ioc_with_approval` (hash/ip/domain/process/path) |

## Defined profile catalog (15)

| Name | Status | Target OS | Category | Risk | Requires approval |
|------|--------|-----------|----------|------|-------------------|
| `windows_basic_triage` | defined | windows | triage | low | no |
| `windows_process_network_triage` | defined | windows | triage | medium | no |
| `windows_persistence_triage` | defined | windows | persistence | medium | no |
| `windows_lateral_movement_triage` | defined | windows | lateral-movement | high | no |
| `windows_ransomware_triage` | defined | windows | ransomware | high | yes |
| `windows_credential_theft_triage` | defined | windows | credential-theft | high | yes |
| `windows_eventlog_triage` | defined | windows | eventlog | medium | no |
| `windows_browser_activity_triage` | defined | windows | browser-activity | high | yes |
| `windows_timeline_triage` | defined | windows | timeline | high | yes |
| `linux_basic_triage` | defined | linux | triage | low | no |
| `linux_process_network_triage` | defined | linux | triage | medium | no |
| `linux_persistence_triage` | defined | linux | persistence | medium | no |
| `ioc_hash_hunt` | defined | any | ioc | low | no |
| `ioc_ip_hunt` | defined | any | ioc | low | no |
| `ioc_domain_hunt` | defined | any | ioc | low | no |

## Authoring a new profile

1. Add `profiles/<name>.yaml` matching `internal/dfir.Profile`'s strict
   YAML shape (`name`, `display_name`, `description`, `target_os`,
   `category`, `risk_level`, `requires_approval`, `max_runtime_seconds`,
   `max_result_rows`, `max_result_bytes`, `artifacts`). Unknown fields are
   rejected so typos such as `max_runtime_sum` fail during profile loading.
2. Verify every artifact name against the actual artifact catalog of the
   target Velociraptor server/version — do not guess names for a
   profile that will really be used.
3. Add each artifact to `policy.allowed_artifacts` in the deployment
   config, and the profile name itself to `policy.allowed_profiles`.
   Both are deliberate, reviewed operator actions.
4. Run `velo_validate_dfir_profile` (once implemented, v0.4.0) before
   relying on the profile in a collection or hunt.

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
