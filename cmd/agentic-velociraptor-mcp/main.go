// Command agentic-velociraptor-mcp is the entrypoint for the
// agentic-velociraptor-mcp MCP server.
//
// As of v0.1.0 this starts a real MCP server over the stdio transport
// (the only transport this project supports; see
// docs/security-model.md), exposing 8 read-only tools. The five
// visibility tools (velo_health_check, velo_search_clients,
// velo_get_client_info, velo_list_artifact_names,
// velo_get_artifact_details) make real Velociraptor gRPC calls when
// velociraptor.read_api_config_path is configured, or run in mock mode
// otherwise. See PROJECT_PLAN.md and PROJECT_STATE.md for what's left.
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
	"syscall"
	"time"

	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/audit"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/config"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/dfir"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/mcpserver"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/policy"
	"github.com/hdyrawan/agentic-velociraptor-mcp/internal/velociraptor"
)

// version is the build version. Overridden at release build time via
// -ldflags "-X main.version=...".
var version = "0.2.0"

// defaultProfilesDir is the --profiles-dir flag's default value.
// resolveProfilesDir only applies its cwd-independent fallback when the
// flag is left at this default; an explicit --profiles-dir is always
// used exactly as given.
const defaultProfilesDir = "profiles"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout))
}

func run(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("agentic-velociraptor-mcp", flag.ContinueOnError)
	fs.SetOutput(out)

	showVersion := fs.Bool("version", false, "print version and exit")
	configPath := fs.String("config", "", "path to server config YAML (see docs/configuration.md)")
	profilesDir := fs.String("profiles-dir", defaultProfilesDir, "path to the DFIR profile YAML directory (see docs/dfir-profiles.md)")

	fs.Usage = func() {
		fmt.Fprintf(out, "agentic-velociraptor-mcp: secure-by-design MCP server for Velociraptor endpoint DFIR\n\n")
		fmt.Fprintf(out, "Usage:\n  agentic-velociraptor-mcp --config /path/to/config.yaml [flags]\n\n")
		fmt.Fprintf(out, "Flags:\n")
		fs.PrintDefaults()
		fmt.Fprintf(out, "\nStatus: v0.1.0. Starts a real MCP server over stdio exposing 8 read-only\n")
		fmt.Fprintf(out, "tools: velo_health_check, velo_search_clients, velo_get_client_info,\n")
		fmt.Fprintf(out, "velo_list_artifact_names, velo_get_artifact_details, velo_list_dfir_profiles,\n")
		fmt.Fprintf(out, "velo_get_dfir_profile, and velo_validate_dfir_profile. The five visibility\n")
		fmt.Fprintf(out, "tools make real Velociraptor gRPC calls when velociraptor.read_api_config_path\n")
		fmt.Fprintf(out, "is set in --config, and otherwise run in mock mode. See PROJECT_PLAN.md and\n")
		fmt.Fprintf(out, "PROJECT_STATE.md.\n")
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
// If velociraptor.read_api_config_path is empty, the read client runs
// in mock mode (Deps.VelociraptorReadMode = "mock") and no network
// connection is attempted. If it is set, buildDeps eagerly loads it and
// constructs a real gRPC client (Deps.VelociraptorReadMode = "real");
// a missing, unreadable, unsafe, or malformed API config file fails
// buildDeps outright rather than silently falling back to mock mode —
// an operator who configured a real connection must be told it's
// broken, not served a quietly-degraded mock. velociraptor.write_api_config_path
// is never read in this milestone: WriteClient is always the mock
// placeholder.
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
	mode := mcpserver.VelociraptorModeMock
	if cfg.Velociraptor.ReadAPIConfigPath != "" {
		timeout := time.Duration(cfg.Velociraptor.TimeoutSeconds) * time.Second
		rc, err := velociraptor.NewGRPCClient(cfg.Velociraptor.ReadAPIConfigPath, cfg.Velociraptor.OrgID, timeout, cfg.Velociraptor.MaxRows)
		if err != nil {
			return mcpserver.Deps{}, "", fmt.Errorf("velociraptor read client: %w", err)
		}
		readClient = rc
		mode = mcpserver.VelociraptorModeReal
	}

	deps := mcpserver.Deps{
		Config:               cfg,
		Policy:               policy.NewEngine(cfg.Policy),
		Audit:                auditSink,
		Profiles:             profiles,
		ReadClient:           readClient,
		VelociraptorReadMode: mode,

		// WriteClient is always the mock placeholder in this milestone:
		// no code path reads WriteAPIConfigPath. TODO(v0.2.0): construct
		// a real write-capable client only once an approval-gated tool
		// exists to use it.
		WriteClient: velociraptor.NewClient(),

		// Approvals is left nil: no approval-gated tool is registered in
		// this release. TODO(v0.2.0): construct a real approval.Store
		// once one exists.
	}

	return deps, cfg.Server.Name, nil
}
