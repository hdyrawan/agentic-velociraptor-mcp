package mcpserver

import (
	"context"
	"sort"
	"strings"
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

// writeCapableTools names the v0.4.0 and v0.6.0 tools that mutate
// Velociraptor endpoint/flow/hunt state (gated by writePilotEnabled and
// an approval reference). Every other registered tool must be read-only.
var writeCapableTools = map[string]bool{
	"velo_collect_artifact_with_approval":     true,
	"velo_collect_dfir_profile_with_approval": true,
	"velo_cancel_flow_with_approval":          true,
	"velo_download_flow_upload_with_approval": true,
	"velo_start_hunt_with_approval":           true,
	"velo_start_dfir_hunt_with_approval":      true,
	"velo_cancel_hunt_with_approval":          true,
}

func TestNewRegistersExactlyTwentySevenTools(t *testing.T) {
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
		"velo_cancel_flow_with_approval",
		"velo_cancel_hunt_with_approval",
		"velo_collect_artifact_with_approval",
		"velo_collect_dfir_profile_with_approval",
		"velo_compare_dfir_profiles",
		"velo_download_flow_upload_with_approval",
		"velo_find_profiles_by_artifact",
		"velo_get_artifact_details",
		"velo_get_client_info",
		"velo_get_dfir_profile",
		"velo_get_flow_results",
		"velo_get_flow_status",
		"velo_get_flow_upload_metadata",
		"velo_get_hunt_results",
		"velo_get_hunt_status",
		"velo_health_check",
		"velo_list_artifact_names",
		"velo_list_dfir_profiles",
		"velo_list_flow_uploads",
		"velo_list_flows",
		"velo_list_hunts",
		"velo_plan_dfir_triage",
		"velo_preview_hunt_scope",
		"velo_search_clients",
		"velo_start_dfir_hunt_with_approval",
		"velo_start_hunt_with_approval",
		"velo_validate_dfir_profile",
	}

	if len(names) != len(want) {
		t.Fatalf("callable tools = %v (len=%d), want exactly %d tools: %v", names, len(names), len(want), want)
	}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("callable tools = %v, want %v", names, want)
			break
		}
	}
}

// TestNewNeverRegistersUnsafeTools guards against regressions where a
// raw-VQL or unapproved-collection tool accidentally becomes callable.
// v0.4.0/v0.6.0 write-capable tools are intentionally registered and
// tracked in writeCapableTools; anything else matching these substrings
// would be a regression.
func TestNewNeverRegistersUnsafeTools(t *testing.T) {
	deps, _ := testDeps(t)
	srv := New("agentic-velociraptor-mcp-test", "0.0.0-test", deps)

	session := connectTestClient(t, srv)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	forbiddenSubstrings := []string{
		"run_vql",
	}

	for _, tool := range res.Tools {
		for _, bad := range forbiddenSubstrings {
			if strings.Contains(strings.ToLower(tool.Name), bad) {
				t.Errorf("tool %q is callable and matches forbidden substring %q", tool.Name, bad)
			}
		}
		if strings.Contains(tool.Name, "collect") || strings.Contains(tool.Name, "cancel") || strings.Contains(tool.Name, "download") || strings.Contains(tool.Name, "start_hunt") || strings.Contains(tool.Name, "start_dfir") {
			if !writeCapableTools[tool.Name] {
				t.Errorf("tool %q matches a write-shaped name but is not in the known writeCapableTools allowlist", tool.Name)
			}
		}
	}
}

func TestNewRegisteredToolsAreNonDestructiveAndClosedWorld(t *testing.T) {
	deps, _ := testDeps(t)
	srv := New("agentic-velociraptor-mcp-test", "0.0.0-test", deps)

	session := connectTestClient(t, srv)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	for _, tool := range res.Tools {
		if tool.Annotations == nil {
			continue // write tools may have nil annotations
		}
		wantReadOnly := !writeCapableTools[tool.Name]
		if tool.Annotations.ReadOnlyHint != wantReadOnly {
			t.Errorf("tool %q: ReadOnlyHint = %v, want %v", tool.Name, tool.Annotations.ReadOnlyHint, wantReadOnly)
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

// TestCallNewVisibilityToolsOverMCPSession confirms the four v0.1.0
// visibility tools are actually callable end to end at the MCP protocol
// level (not just registered), each in mock mode.
func TestCallNewVisibilityToolsOverMCPSession(t *testing.T) {
	deps, _ := testDeps(t)
	srv := New("agentic-velociraptor-mcp-test", "0.0.0-test", deps)

	session := connectTestClient(t, srv)

	cases := []struct {
		name string
		args map[string]any
	}{
		{name: "velo_search_clients", args: map[string]any{}},
		{name: "velo_get_client_info", args: map[string]any{"client_id": "C.1234abcd5678ef90"}},
		{name: "velo_list_artifact_names", args: map[string]any{}},
		{name: "velo_get_artifact_details", args: map[string]any{"name": "Generic.Client.Info"}},
	}

	for _, tc := range cases {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil {
			t.Fatalf("CallTool %s: %v", tc.name, err)
		}
		if res.IsError {
			t.Fatalf("CallTool %s returned IsError=true: %+v", tc.name, res)
		}
	}
}

// TestCallCollectionToolsBlockedByDefaultOverMCPSession confirms every
// v0.4.0 write-capable tool is registered and callable at the MCP
// protocol level, but refuses to do anything beyond reporting itself
// disabled under the default read-only/no-approval-store testDeps
// configuration — the write pilot must never be silently on.
func TestCallCollectionToolsBlockedByDefaultOverMCPSession(t *testing.T) {
	deps, _ := testDeps(t)
	srv := New("agentic-velociraptor-mcp-test", "0.0.0-test", deps)

	session := connectTestClient(t, srv)

	cases := []struct {
		name string
		args map[string]any
	}{
		{name: "velo_collect_artifact_with_approval", args: map[string]any{
			"client_id": "C.1234abcd5678ef90", "artifact": "Generic.Client.Info",
			"case_id": "CASE-1", "reason": "triage", "requester": "analyst", "approval_reference": "ref-1",
		}},
		{name: "velo_collect_dfir_profile_with_approval", args: map[string]any{
			"client_id": "C.1234abcd5678ef90", "profile": "windows_basic_triage",
			"case_id": "CASE-1", "reason": "triage", "requester": "analyst", "approval_reference": "ref-1",
		}},
		{name: "velo_cancel_flow_with_approval", args: map[string]any{
			"client_id": "C.1234abcd5678ef90", "flow_id": "F.BN2HJC4N4T6KG",
			"case_id": "CASE-1", "reason": "stop it", "requester": "analyst", "approval_reference": "ref-1",
		}},
		{name: "velo_download_flow_upload_with_approval", args: map[string]any{
			"client_id": "C.1234abcd5678ef90", "flow_id": "F.BN2HJC4N4T6KG", "upload_name": "file.bin",
			"case_id": "CASE-1", "reason": "evidence", "requester": "analyst", "approval_reference": "ref-1",
		}},
	}

	for _, tc := range cases {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil {
			t.Fatalf("CallTool %s: unexpected transport error: %v", tc.name, err)
		}
		if !res.IsError {
			t.Errorf("CallTool %s: IsError = false, want true (write pilot must be disabled by default)", tc.name)
		}
	}
}

// TestCallReadOnlyFlowUploadToolsOverMCPSession confirms the two v0.4.0
// read-only upload tools are callable regardless of write-pilot state.
func TestCallReadOnlyFlowUploadToolsOverMCPSession(t *testing.T) {
	deps, _ := testDeps(t)
	srv := New("agentic-velociraptor-mcp-test", "0.0.0-test", deps)

	session := connectTestClient(t, srv)

	cases := []struct {
		name string
		args map[string]any
	}{
		{name: "velo_list_flow_uploads", args: map[string]any{"client_id": "C.1234abcd5678ef90", "flow_id": "F.BN2HJC4N4T6KG"}},
		{name: "velo_get_flow_upload_metadata", args: map[string]any{"client_id": "C.1234abcd5678ef90", "flow_id": "F.BN2HJC4N4T6KG", "upload_name": "file.bin"}},
	}

	for _, tc := range cases {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil {
			t.Fatalf("CallTool %s: %v", tc.name, err)
		}
		if res.IsError {
			t.Fatalf("CallTool %s returned IsError=true: %+v", tc.name, res)
		}
	}
}

func TestCallWorkflowToolsOverMCPSession(t *testing.T) {
	deps, _ := testDeps(t)
	srv := New("agentic-velociraptor-mcp-test", "0.0.0-test", deps)

	session := connectTestClient(t, srv)

	cases := []struct {
		name string
		args map[string]any
	}{
		{name: "velo_plan_dfir_triage", args: map[string]any{"case_type": "ransomware", "target_os": "windows"}},
		{name: "velo_compare_dfir_profiles", args: map[string]any{"names": []any{"windows_basic_triage", "windows_process_network_triage"}}},
		{name: "velo_find_profiles_by_artifact", args: map[string]any{"artifact": "Generic.Client.Info"}},
	}

	for _, tc := range cases {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil {
			t.Fatalf("CallTool %s: %v", tc.name, err)
		}
		if res.IsError {
			t.Fatalf("CallTool %s returned IsError=true: %+v", tc.name, res)
		}
	}
}

func TestCallFlowToolsOverMCPSession(t *testing.T) {
	deps, _ := testDeps(t)
	srv := New("agentic-velociraptor-mcp-test", "0.0.0-test", deps)

	session := connectTestClient(t, srv)

	cases := []struct {
		name string
		args map[string]any
	}{
		{name: "velo_list_flows", args: map[string]any{"client_id": testClientID}},
		{name: "velo_get_flow_status", args: map[string]any{"client_id": testClientID, "flow_id": testFlowID}},
		{name: "velo_get_flow_results", args: map[string]any{"client_id": testClientID, "flow_id": testFlowID}},
	}

	for _, tc := range cases {
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		if err != nil {
			t.Fatalf("CallTool %s: %v", tc.name, err)
		}
		if res.IsError {
			t.Fatalf("CallTool %s returned IsError=true: %+v", tc.name, res)
		}
	}
}
