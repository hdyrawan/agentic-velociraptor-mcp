# DFIR profiles

Status: 15 of 15 planned profile definitions exist under `profiles/`. All are defined, though some require specific approval for high-sensitivity cases (e.g., credential_theft, browser_activity).
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

## Planned profile catalog (15)

| Name | Status | Target OS | Category |
|------|--------|-----------|----------|
| `windows_basic_triage` | defined | windows | triage |
| `windows_process_network_triage` | planned | windows | triage |
| `windows_persistence_triage` | planned | windows | persistence |
| `windows_lateral_movement_triage` | planned | windows | lateral-movement |
| `windows_ransomware_triage` | defined | windows | ransomware |
| `windows_credential_theft_triage` | planned | windows | credential-theft |
| `windows_eventlog_triage` | planned | windows | eventlog |
| `windows_browser_activity_triage` | planned | windows | browser-activity |
| `windows_timeline_triage` | planned | windows | timeline |
| `linux_basic_triage` | defined | linux | triage |
| `linux_process_network_triage` | planned | linux | triage |
| `linux_persistence_triage` | planned | linux | persistence |
| `ioc_hash_hunt` | planned | any | ioc |
| `ioc_ip_hunt` | planned | any | ioc |
| `ioc_domain_hunt` | planned | any | ioc |

## Authoring a new profile

1. Add `profiles/<name>.yaml` matching `internal/dfir.Profile`'s YAML
   shape (`name`, `display_name`, `description`, `target_os`, `category`,
   `artifacts`).
2. Verify every artifact name against the actual artifact catalog of the
   target Velociraptor server/version — do not guess names for a
   profile that will really be used.
3. Add each artifact to `policy.allowed_artifacts` in the deployment
   config, and the profile name itself to `policy.allowed_profiles`.
   Both are deliberate, reviewed operator actions.
4. Run `velo_validate_dfir_profile` (once implemented, v0.4.0) before
   relying on the profile in a collection or hunt.

## IOC profiles specifically

`ioc_hash_hunt`, `ioc_ip_hunt`, and `ioc_domain_hunt` back
`velo_hunt_ioc_with_approval` together with `internal/vql`'s fixed
templates (`TemplateIOCHashHunt`, `TemplateIOCIPHunt`,
`TemplateIOCDomainHunt`). The indicator value itself is validated by
`internal/validation` (`Hash`, `IP`, `Domain`) and passed as a bound
parameter — never concatenated into a query.
