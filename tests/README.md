# tests/

Reserved for integration-level tests that exercise multiple packages
together (e.g. config load → policy engine → dfir registry) or, once
v0.1.0-alpha.1 lands, MCP protocol-level tests against a running server
instance.

Package-level unit tests live alongside their code as `*_test.go` files
(e.g. `internal/velociraptor/client_test.go`), per standard Go
convention, not here.

No integration tests exist yet as of v0.0.x.
