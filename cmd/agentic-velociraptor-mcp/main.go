// Command agentic-velociraptor-mcp is the entrypoint for the
// agentic-velociraptor-mcp MCP server.
//
// As of v0.4.0 (rebased onto v0.5.0's read-only flow/result backfill)
// this starts a real MCP server over the stdio transport (the only
// transport this project supports; see docs/security-model.md),
// exposing 20 tools: the 14 read-only tools from v0.1.0-v0.5.0
// (visibility, flow/result, DFIR profile, and workflow tools), plus six
// new tools implementing a controlled, auditable, single-client
// collection pilot (velo_collect_artifact_with_approval,
// velo_collect_dfir_profile_with_approval, velo_cancel_flow_with_approval,
// velo_list_flow_uploads, velo_get_flow_upload_metadata,
// velo_download_flow_upload_with_approval). See PROJECT_PLAN.md and
// PROJECT_STATE.md for what's left.
//
// This binary also provides a second, non-MCP entrypoint: the `approve`
// subcommand, run directly by a human operator (never reachable over the
// MCP stdio transport, and never callable by any MCP client, including
// an LLM driving one) to create and decide an approval.Request against
// the same on-disk approval.Store the running MCP server reads. This is
// what makes "approval" a real control rather than theater: an MCP
// session can request that a write-capable tool run, by supplying an
// approval_reference, but only a human running this separate command
// line can make that reference valid. See docs/approval-flow.md.
//
// Requires Go 1.25+ (the official MCP Go SDK dependency's minimum).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/approval"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/config"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/dfir"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/mcpserver"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/policy"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

// version is the build version. Overridden at release build time via
// -ldflags "-X main.version=...".
var version = "0.8.0"

// defaultProfilesDir is the --profiles-dir flag's default value.
// resolveProfilesDir only applies its cwd-independent fallback when the
// flag is left at this default; an explicit --profiles-dir is always
// used exactly as given.
const defaultProfilesDir = "profiles"

// defaultApprovalTTLSeconds is the approve CLI's default --ttl-seconds,
// matching config.Default()'s Approval.TTLSeconds.
const defaultApprovalTTLSeconds = 900

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(args []string, out io.Writer) int {
	if len(args) > 0 && args[0] == "approve" {
		return runApprove(args[1:], out)
	}
	return runServer(args, out)
}

func runServer(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("agentic-velociraptor-mcp", flag.ContinueOnError)
	fs.SetOutput(out)

	showVersion := fs.Bool("version", false, "print version and exit")
	configPath := fs.String("config", "", "path to server config YAML (see docs/configuration.md)")
	profilesDir := fs.String("profiles-dir", defaultProfilesDir, "path to the DFIR profile YAML directory (see docs/dfir-profiles.md)")

	fs.Usage = func() {
		fmt.Fprintf(out, "agentic-velociraptor-mcp: secure-by-design MCP server for Velociraptor endpoint DFIR\n\n")
		fmt.Fprintf(out, "Usage:\n  agentic-velociraptor-mcp --config /path/to/config.yaml [flags]\n")
		fmt.Fprintf(out, "  agentic-velociraptor-mcp approve --store PATH --reference REF ... (see 'approve -h')\n\n")
		fmt.Fprintf(out, "Flags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(out, "\nStatus: v0.4.0. Starts a real MCP server over stdio exposing 20 tools: the 14\n")
		fmt.Fprintf(out, "read-only tools from v0.1.0-v0.5.0, plus a controlled, approval-gated\n")
		fmt.Fprintf(out, "collection pilot (collect artifact, collect DFIR profile, cancel flow, list/get\n")
		fmt.Fprintf(out, "flow upload metadata, download flow upload). Every approval-gated tool is\n")
		fmt.Fprintf(out, "disabled unless policy.mode is \"controlled\" and approval.store_path is set; no\n")
		fmt.Fprintf(out, "MCP tool can create or decide an approval, only the separate 'approve'\n")
		fmt.Fprintf(out, "subcommand can. This is a controlled pilot, not unrestricted Velociraptor write\n")
		fmt.Fprintf(out, "access: no hunts, no multi-client collection, no raw VQL. See PROJECT_PLAN.md,\n")
		fmt.Fprintf(out, "PROJECT_STATE.md, and docs/approval-flow.md.\n")
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	profilesDirExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "profiles-dir" {
			profilesDirExplicit = true
		}
	})

	if *showVersion {
		fmt.Fprintf(out, "agentic-velociraptor-mcp %s\n", version)
		return 0
	}

	if *configPath == "" {
		fs.Usage()
		return 2
	}

	deps, name, err := buildDeps(*configPath, resolveProfilesDir(*profilesDir, profilesDirExplicit))
	if err != nil {
		fmt.Fprintf(out, "agentic-velociraptor-mcp: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	srv := mcpserver.New(name, version, deps)
	if err := srv.Run(ctx); err != nil {
		fmt.Fprintf(out, "agentic-velociraptor-mcp: %v\n", err)
		return 1
	}
	return 0
}

// resolveProfilesDir makes the default --profiles-dir value less
// dependent on the process's working directory, without changing
// behavior for anyone relying on the existing cwd-relative lookup or
// passing an explicit path.
//
// If explicit is true (the caller passed --profiles-dir), dir is
// returned unchanged: an explicit operator choice is never
// second-guessed. Otherwise, if dir does not resolve to a directory
// relative to the current working directory, this also tries dir
// relative to the running executable's own directory (e.g. so
// `/opt/agentic-velociraptor-mcp/agentic-velociraptor-mcp` finds
// `/opt/agentic-velociraptor-mcp/profiles` even when invoked from
// somewhere else). If neither resolves, dir is returned unchanged and
// internal/dfir.LoadDir will fail with its normal, clear error.
func resolveProfilesDir(dir string, explicit bool) string {
	if explicit {
		return dir
	}
	if isDir(dir) {
		return dir
	}
	if exe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exe), dir)
		if isDir(candidate) {
			return candidate
		}
	}
	return dir
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// buildDeps loads and validates configuration, then constructs every
// dependency mcpserver.Deps needs. It never logs the contents of
// configPath or the Velociraptor API config paths it references.
//
// If velociraptor.read_api_config_path is empty, the read client runs in
// mock mode (Deps.VelociraptorReadMode = "mock") and no network
// connection is attempted; velociraptor.write_api_config_path behaves
// identically for Deps.WriteClient/VelociraptorWriteMode. A missing,
// unreadable, unsafe, or malformed API config file fails buildDeps
// outright rather than silently falling back to mock mode — an operator
// who configured a real connection must be told it's broken, not served
// a quietly-degraded mock.
//
// Deps.Approvals is nil unless approval.store_path is configured, in
// which case an approval.FileStore is constructed against it. Combined
// with policy.mode, this is the two-condition gate
// mcpserver.writePilotEnabled checks before any write-capable tool does
// anything but report itself disabled.
func buildDeps(configPath, profilesDir string) (mcpserver.Deps, string, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return mcpserver.Deps{}, "", err
	}
	if err := config.Validate(cfg); err != nil {
		return mcpserver.Deps{}, "", fmt.Errorf("invalid config: %w", err)
	}

	profiles, err := dfir.LoadDir(profilesDir)
	if err != nil {
		return mcpserver.Deps{}, "", err
	}

	var auditSink audit.Sink
	if cfg.Audit.Enabled {
		auditSink, err = audit.NewJSONLWriter(cfg.Audit.Path)
		if err != nil {
			return mcpserver.Deps{}, "", err
		}
	} else {
		auditSink = audit.NopSink{}
	}

	readClient := velociraptor.Client(velociraptor.NewClient())
	readMode := mcpserver.VelociraptorModeMock
	if cfg.Velociraptor.ReadAPIConfigPath != "" {
		timeout := time.Duration(cfg.Velociraptor.TimeoutSeconds) * time.Second
		rc, err := velociraptor.NewGRPCClient(cfg.Velociraptor.ReadAPIConfigPath, cfg.Velociraptor.OrgID, timeout, cfg.Velociraptor.MaxRows)
		if err != nil {
			return mcpserver.Deps{}, "", fmt.Errorf("velociraptor read client: %w", err)
		}
		readClient = rc
		readMode = mcpserver.VelociraptorModeReal
	}

	writeClient := velociraptor.Client(velociraptor.NewClient())
	writeMode := mcpserver.VelociraptorModeMock
	if cfg.Velociraptor.WriteAPIConfigPath != "" {
		timeout := time.Duration(cfg.Velociraptor.TimeoutSeconds) * time.Second
		wc, err := velociraptor.NewGRPCClient(cfg.Velociraptor.WriteAPIConfigPath, cfg.Velociraptor.OrgID, timeout, cfg.Velociraptor.MaxRows)
		if err != nil {
			return mcpserver.Deps{}, "", fmt.Errorf("velociraptor write client: %w", err)
		}
		writeClient = wc
		writeMode = mcpserver.VelociraptorModeReal
	}

	var approvals approval.Store
	if cfg.Approval.StorePath != "" {
		ttl := time.Duration(cfg.Approval.TTLSeconds) * time.Second
		store, err := approval.NewFileStore(cfg.Approval.StorePath, ttl)
		if err != nil {
			return mcpserver.Deps{}, "", fmt.Errorf("approval store: %w", err)
		}
		approvals = store
	}

	deps := mcpserver.Deps{
		Config:                cfg,
		Policy:                policy.NewEngine(cfg.Policy),
		Audit:                 auditSink,
		Approvals:             approvals,
		Profiles:              profiles,
		ReadClient:            readClient,
		VelociraptorReadMode:  readMode,
		WriteClient:           writeClient,
		VelociraptorWriteMode: writeMode,
	}

	return deps, cfg.Server.Name, nil
}

// stringSliceFlag accumulates repeated -param key=value flags into a
// map, for runApprove's collect_artifact operation.
type paramMapFlag struct {
	values map[string]string
}

func (f *paramMapFlag) String() string {
	if f == nil {
		return ""
	}
	parts := make([]string, 0, len(f.values))
	for k, v := range f.values {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (f *paramMapFlag) Set(s string) error {
	k, v, ok := strings.Cut(s, "=")
	if !ok {
		return fmt.Errorf("expected key=value, got %q", s)
	}
	if f.values == nil {
		f.values = map[string]string{}
	}
	f.values[k] = v
	return nil
}

// stringSliceFlag accumulates repeated flag occurrences into a slice,
// for runApprove's hunt-scope client ID list.
type stringSliceFlag struct {
	values []string
}

func (f *stringSliceFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(f.values, ",")
}

func (f *stringSliceFlag) Set(s string) error {
	f.values = append(f.values, s)
	return nil
}

// runApprove implements the `approve` subcommand: a human operator's
// direct entrypoint for creating and deciding an approval.Request. It is
// never invoked by the MCP server and has no MCP tool counterpart.
func runApprove(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("agentic-velociraptor-mcp approve", flag.ContinueOnError)
	fs.SetOutput(out)

	store := fs.String("store", "", "path to the approval store JSON file (must match the running MCP server's approval.store_path)")
	reference := fs.String("reference", "", "approval reference to create/decide, e.g. a ticket number")
	operation := fs.String("operation", "", "one of: collect_artifact, collect_dfir_profile, cancel_flow, download_flow_upload, start_hunt, start_dfir_hunt, cancel_hunt, hunt_ioc")
	caseID := fs.String("case-id", "", "investigation/case identifier")
	reason := fs.String("reason", "", "justification for this operation")
	requester := fs.String("requester", "", "identity of whoever is asking for this operation")
	approvedBy := fs.String("approved-by", "", "identity of the human deciding this request (required)")
	note := fs.String("note", "", "optional note recorded with the decision")
	deny := fs.Bool("deny", false, "deny the request instead of approving it")
	ttlSeconds := fs.Int("ttl-seconds", defaultApprovalTTLSeconds, "approval time-to-live in seconds, must match approval.ttl_seconds")

	clientID := fs.String("client-id", "", "target client ID (collect_artifact, collect_dfir_profile, cancel_flow, download_flow_upload)")
	artifact := fs.String("artifact", "", "target artifact name (collect_artifact, start_hunt)")
	profile := fs.String("profile", "", "target DFIR profile name (collect_dfir_profile, start_dfir_hunt)")
	flowID := fs.String("flow-id", "", "target flow ID (cancel_flow, download_flow_upload)")
	uploadName := fs.String("upload-name", "", "target upload name (download_flow_upload)")
	huntID := fs.String("hunt-id", "", "target hunt ID (cancel_hunt)")
	label := fs.String("label", "", "hunt scope label filter (start_hunt, start_dfir_hunt, hunt_ioc)")
	targetAll := fs.Bool("all", false, "hunt scope targets all clients (start_hunt, start_dfir_hunt, hunt_ioc)")
	iocKind := fs.String("ioc-kind", "", "indicator kind: hash, ip, domain, process, or path (hunt_ioc)")
	iocValue := fs.String("ioc-value", "", "indicator value to hunt for (hunt_ioc)")
	params := &paramMapFlag{}
	fs.Var(params, "param", "artifact parameter as key=value; may be repeated (collect_artifact, start_hunt)")
	clientIDs := &stringSliceFlag{}
	fs.Var(clientIDs, "hunt-client-id", "explicit hunt scope client ID; may be repeated (start_hunt, start_dfir_hunt, hunt_ioc)")

	fs.Usage = func() {
		fmt.Fprintf(out, "agentic-velociraptor-mcp approve: create and decide an approval.Request out-of-band\n\n")
		fmt.Fprintf(out, "This is a human-operator-only command. It is never called by the MCP server\n")
		fmt.Fprintf(out, "and has no MCP tool equivalent, so no MCP client can grant its own approval.\n\n")
		fmt.Fprintf(out, "Usage:\n  agentic-velociraptor-mcp approve --store PATH --reference REF --operation OP \\\n")
		fmt.Fprintf(out, "    --case-id ID --reason TEXT --requester WHO --approved-by WHO [target flags] [--deny]\n\n")
		fmt.Fprintf(out, "Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	req := approval.Request{
		ID:        *reference,
		Operation: approval.Operation(*operation),
		CaseID:    *caseID,
		Reason:    *reason,
		Requester: *requester,
	}

	switch req.Operation {
	case approval.OperationCollectArtifact:
		req.ClientID = *clientID
		req.Artifact = *artifact
		req.Parameters = params.values
	case approval.OperationCollectDFIRProfile:
		req.ClientID = *clientID
		req.Profile = *profile
	case approval.OperationCancelFlow:
		req.ClientID = *clientID
		req.FlowID = *flowID
	case approval.OperationDownloadFlowUpload:
		req.ClientID = *clientID
		req.FlowID = *flowID
		req.UploadName = *uploadName
	case approval.OperationStartHunt:
		req.Artifact = *artifact
		req.Parameters = params.values
		req.ClientIDs = clientIDs.values
		req.Label = *label
		req.TargetAll = *targetAll
	case approval.OperationStartDFIRHunt:
		req.Profile = *profile
		req.ClientIDs = clientIDs.values
		req.Label = *label
		req.TargetAll = *targetAll
	case approval.OperationCancelHunt:
		req.HuntID = *huntID
	case approval.OperationHuntIOC:
		// hunt_ioc approvals must fingerprint-match what
		// velo_hunt_ioc_with_approval will verify at execution time, so
		// the request is built through the exact same validation and
		// template-binding path the MCP handler uses (ValidateHuntScope,
		// ValidateIOC, template lookup, vql.Bind) — never from raw
		// --artifact/--param flags.
		built, err := mcpserver.BuildHuntIOCApprovalRequest(*caseID, *reason, *requester, *iocKind, *iocValue, clientIDs.values, *label, *targetAll)
		if err != nil {
			fmt.Fprintf(out, "agentic-velociraptor-mcp approve: hunt_ioc: %v\n", err)
			return 2
		}
		built.ID = *reference
		req = built
	default:
		fmt.Fprintf(out, "agentic-velociraptor-mcp approve: --operation must be one of collect_artifact, collect_dfir_profile, cancel_flow, download_flow_upload, start_hunt, start_dfir_hunt, cancel_hunt, hunt_ioc (got %q)\n", *operation)
		return 2
	}

	if *store == "" {
		fmt.Fprintf(out, "agentic-velociraptor-mcp approve: --store is required\n")
		return 2
	}

	fileStore, err := approval.NewFileStore(*store, time.Duration(*ttlSeconds)*time.Second)
	if err != nil {
		fmt.Fprintf(out, "agentic-velociraptor-mcp approve: %v\n", err)
		return 1
	}

	ctx := context.Background()
	created, err := fileStore.Create(ctx, req)
	if err != nil {
		fmt.Fprintf(out, "agentic-velociraptor-mcp approve: create request: %v\n", err)
		return 1
	}

	dec := approval.Decision{
		RequestID:  created.ID,
		Approved:   !*deny,
		ApprovedBy: *approvedBy,
		Note:       *note,
	}
	if err := fileStore.Decide(ctx, dec); err != nil {
		fmt.Fprintf(out, "agentic-velociraptor-mcp approve: decide request: %v\n", err)
		return 1
	}

	verb := "approved"
	if *deny {
		verb = "denied"
	}
	fmt.Fprintf(out, "agentic-velociraptor-mcp approve: reference %q (%s) %s by %s\n", created.ID, req.Operation, verb, *approvedBy)
	return 0
}
