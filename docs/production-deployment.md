# Production deployment guide

Status: placeholder. This document is scheduled for v0.5.0/v1.0.0 and
must not be treated as sufficient for a real deployment until then. Do
not deploy this server against a production Velociraptor instance based
on the v0.0.x skeleton.

## Prerequisites (once implemented)

- Completion of [lab-validation-plan.md](lab-validation-plan.md) against
  a disposable environment matching production topology.
- Velociraptor reader and investigator API identities provisioned per
  [velociraptor-permissions.md](velociraptor-permissions.md), reviewed by
  whoever owns Velociraptor server administration.
- `policy.allowed_artifacts` / `policy.allowed_profiles` reviewed and
  signed off by the DFIR team that will rely on this server, not left at
  defaults.
- An approval mechanism (see [approval-flow.md](approval-flow.md)) wired
  to real humans, not a stub.
- Audit log storage with retention and access control matching your
  organization's investigation-record requirements.

## Planned topics (v0.5.0)

- Container image (non-root user, minimal base, read-only root
  filesystem where possible).
- Secrets management for `read_api_config_path` /
  `write_api_config_path` (e.g. mounted from a secret store, not baked
  into the image).
- Process supervision / restart policy for the stdio MCP server process,
  and how it's wired to whatever MCP client host runs alongside it.
- Log/audit shipping to central storage.
- Upgrade procedure (config compatibility across versions, migration of
  `profiles/*.yaml`).
- Incident response: what to do if the write API identity's credentials
  are suspected compromised (revoke in Velociraptor first, this server
  has no independent revocation mechanism).

## Non-goals

- This server does not manage Velociraptor server installation,
  upgrades, or its own TLS/network exposure — it is a client of an
  already-operated Velociraptor deployment.
