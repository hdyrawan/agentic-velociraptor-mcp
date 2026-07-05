package policy

// DangerousVelociraptorPermissions lists Velociraptor ACL permissions
// that the MCP API identity (read or write) must never hold. This is
// used by config/deployment validation and documentation generation, not
// by runtime request evaluation (Velociraptor itself enforces its ACLs;
// this list is a checklist for operators setting up API client certs).
//
// See docs/velociraptor-permissions.md for the full rationale per
// permission.
var DangerousVelociraptorPermissions = []string{
	"administrator",
	"ARTIFACT_WRITER",
	"SERVER_ARTIFACT_WRITER",
	"EXECVE",
	"FILESYSTEM_WRITE",
	"SERVER_ADMIN",
}

// TODO(v0.1.0+): once internal/velociraptor can introspect the
// permissions granted to the loaded API config's certificate (if
// Velociraptor exposes that via the gRPC API), add a startup check that
// warns or refuses to start if the read or write identity holds any
// DangerousVelociraptorPermissions entry. Until then this list is
// documentation-only.
