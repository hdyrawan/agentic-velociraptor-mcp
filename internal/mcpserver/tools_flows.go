package mcpserver

// FlowTools cover read access to collection flow status/results/uploads,
// plus the one approval-gated upload download. Listing and status/result
// retrieval are read-only; download is the exception because it
// discloses raw evidence bytes.
//
// TODO(v0.1.0): implement the read-only members against
// internal/velociraptor.FlowReader / UploadReader, enforcing MaxRows and
// MaxResultBytes.
// TODO(v0.2.0): implement velo_download_flow_upload_with_approval against
// UploadDownloader, enforcing MaxUploadBytes and a prior approval.Decision.
var FlowTools = []ToolSpec{
	{
		Name:        "velo_list_flows",
		Description: "List collection flows for a client.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_flow_status",
		Description: "Get the state of one flow (running/finished/error/cancelled).",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_flow_results",
		Description: "Get result rows for one flow, bounded by max_rows/max_result_bytes; response states explicitly if truncated.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_list_flow_uploads",
		Description: "List uploaded files attached to a flow's results, without content.",
		ReadOnly:    true,
	},
	{
		Name:        "velo_get_flow_upload_metadata",
		Description: "Get size/hash metadata for one flow upload, without content.",
		ReadOnly:    true,
	},
	{
		Name:             "velo_download_flow_upload_with_approval",
		Description:      "Download bytes of one flow upload, bounded by max_upload_bytes. Requires prior approval; treated as evidence disclosure, not a read.",
		RequiresApproval: true,
	},
}
