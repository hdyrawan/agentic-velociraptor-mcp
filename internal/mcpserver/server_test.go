package mcpserver

import (
	"context"
	"sort"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// connectTestClient starts srv over an in-memory transport pair and
// returns a connected client session, without touching stdin/stdout or
// spawning any process. This is how tool handlers are exercised at the
// MCP protocol level in tests; see docs/lab-validation-plan.md for the
// separate, real-process smoke test performed manually with MCP
// Inspector / CommandTransport.
func connectTestClient(t *testing.T, srv *Server) *mcp.ClientSession {
	t.Helper()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	ctx := context.Background()
	if _, err := srv.mcp.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server Connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	return session
}

func TestNewRegistersExactlyFourSafeTools(t *testing.T) {
	deps, _ := testDeps(t)
	srv := New("agentic-velociraptor-mcp-test", "0.0.0-test", deps)

	session := connectTestClient(t, srv)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	var names []string
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	want := []string{
		"velo_get_dfir_profile",
		"velo_health_check",
		"velo_list_dfir_profiles",
		"velo_validate_dfir_profile",
	}

	if len(names) != len(want) {
		t.Fatalf("callable tools = %v, want exactly %v", names, want)
	}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("callable tools = %v, want %v", names, want)
			break
		}
	}
}

func TestNewRegisteredToolsAreReadOnlyNonDestructive(t *testing.T) {
	deps, _ := testDeps(t)
	srv := New("agentic-velociraptor-mcp-test", "0.0.0-test", deps)

	session := connectTestClient(t, srv)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	for _, tool := range res.Tools {
		if tool.Annotations == nil {
			t.Errorf("tool %q: missing annotations", tool.Name)
			continue
		}
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q: ReadOnlyHint = false, want true", tool.Name)
		}
		if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
			t.Errorf("tool %q: DestructiveHint = %v, want false", tool.Name, tool.Annotations.DestructiveHint)
		}
		if tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Errorf("tool %q: OpenWorldHint = %v, want false", tool.Name, tool.Annotations.OpenWorldHint)
		}
	}
}

func TestCallHealthCheckOverMCPSession(t *testing.T) {
	deps, _ := testDeps(t)
	srv := New("agentic-velociraptor-mcp-test", "0.0.0-test", deps)

	session := connectTestClient(t, srv)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "velo_health_check"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool velo_health_check returned IsError=true: %+v", res)
	}
}
