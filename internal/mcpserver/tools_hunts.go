package mcpserver

// HuntTools manage multi-client hunts. Preview, listing, status, and
// results are read-only; starting/cancelling a hunt is approval-gated
// and must always be preceded by a scope preview so the approver sees
// blast radius (matched client count) before deciding.
//
// TODO(v0.3.0): implement against internal/velociraptor.HuntReader /
// HuntWriter, enforcing policy.Engine.MaxHuntClients and
// TargetAllAllowed via validation.ValidateHuntScope before any write
// call.
var HuntTools = []ToolSpec{
	{
		Name:        "velo_preview_hunt_scope",
		Description: "Resolve a proposed hunt scope (client IDs, label, or all) against the current client population without starting anything.",
		ReadOnly:    true,
	},
	{
		Name:             "velo_start_hunt_with_approval",
		Description:      "Start a hunt for one allowlisted artifact across a previewed, bounded scope. Requires approval.",
		RequiresApproval: true,
	},
	{
		Name:             "velo_start_dfir_hunt_with_approval",
		Description:      "Start a hunt for every artifact in one allowlisted DFIR profile across a previewed, bounded scope. Requires approval.",
		RequiresApproval: true,
	},
	{
		Name:        "velo_list_hunts",
		Description: "List hunts.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_hunt_status",
		Description: "Get the state and client count of one hunt.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_hunt_results",
		Description: "Get result rows for one hunt, bounded by max_rows/max_result_bytes.",
		ReadOnly:    true,
	},
	{
		Name:             "velo_cancel_hunt_with_approval",
		Description:      "Stop a running hunt. Requires approval.",
		RequiresApproval: true,
	},
}
