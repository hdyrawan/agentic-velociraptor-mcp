package mcpserver

// CollectionTools start or stop artifact collection on a single client.
// Every member here mutates endpoint-facing state and therefore requires
// approval and a case ID/reason, per approval.Request.
//
// TODO(v0.2.0): implement against internal/velociraptor.FlowWriter,
// gated by policy.Engine.ArtifactAllowed / ProfileAllowed and a
// confirmed approval.Decision whose approval.RequestFingerprint matches
// the actual call being made.
var CollectionTools = []ToolSpec{
	{
		Name:             "velo_collect_artifact_with_approval",
		Description:      "Collect one allowlisted artifact from one client. Requires case ID, reason, and approval.",
		RequiresApproval: true,
	},
	{
		Name:             "velo_collect_dfir_profile_with_approval",
		Description:      "Collect every artifact in one allowlisted DFIR profile from one client. Requires case ID, reason, and approval.",
		RequiresApproval: true,
	},
	{
		Name:             "velo_cancel_flow_with_approval",
		Description:      "Cancel a running flow on a client. Requires approval.",
		RequiresApproval: true,
	},
}
