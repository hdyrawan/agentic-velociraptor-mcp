package mcpserver

// IOCTools expose the single IOC-hunting convenience tool. This is the
// only place a hash/IP/domain literal enters a hunt; it is built on top
// of HuntTools and vql.KnownTemplates, never on raw VQL, and remains
// approval-gated because it starts a hunt.
//
// TODO(v0.4.0): implement against internal/validation.Hash / IP / Domain
// for input validation, internal/vql.Bind for template resolution to a
// concrete artifact, and internal/velociraptor.HuntWriter to start the
// hunt (with a preview step first, same as velo_start_hunt_with_approval).
var IOCTools = []ToolSpec{
	{
		Name:             "velo_hunt_ioc_with_approval",
		Description:      "Hunt for a validated hash, IP, or domain indicator across a previewed, bounded scope using a fixed IOC template. Requires approval.",
		RequiresApproval: true,
	},
}
