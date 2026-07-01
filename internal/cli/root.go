package cli

import (
	"context"
	"log/slog"
	"os"

	"github.com/agentfence/agentfence/internal/agent"
	"github.com/agentfence/agentfence/internal/app"
	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/execx"
	"github.com/agentfence/agentfence/internal/logging"
	"github.com/agentfence/agentfence/internal/policy"
	"github.com/agentfence/agentfence/internal/ports"
	"github.com/agentfence/agentfence/internal/project"
	"github.com/agentfence/agentfence/internal/sandbox"
	"github.com/agentfence/agentfence/internal/scanner"
	"github.com/agentfence/agentfence/internal/state"
	sqlitestore "github.com/agentfence/agentfence/internal/storage/sqlite"
	"github.com/agentfence/agentfence/internal/workspace"
	"github.com/spf13/cobra"
)

type rootOptions struct {
	configPath string
	json       bool
	logLevel   string
}

type runtimeDeps struct {
	cfg          config.Config
	paths        state.Paths
	store        *sqlitestore.Store
	git          *project.Git
	runService   *app.RunService
	scanService  *app.ScanService
	applyService *app.ApplyService
	cleanService *app.CleanService
	doctor       *app.DoctorService
	redactor     *execx.Redactor
	logger       *slog.Logger
}

func NewRootCommand() *cobra.Command {
	opts := &rootOptions{}
	cmd := &cobra.Command{
		Use:           "agentfence",
		Short:         "Run AI coding agents in a temporary shadow workspace",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&opts.configPath, "config", "", "config path")
	cmd.PersistentFlags().BoolVar(&opts.json, "json", false, "json output")
	cmd.PersistentFlags().StringVar(&opts.logLevel, "log-level", "info", "log level")
	cmd.AddCommand(newInitCommand(opts))
	cmd.AddCommand(newDoctorCommand(opts))
	cmd.AddCommand(newRunCommand(opts))
	cmd.AddCommand(newScanCommand(opts))
	cmd.AddCommand(newDiffCommand(opts))
	cmd.AddCommand(newApplyCommand(opts))
	cmd.AddCommand(newStatusCommand(opts))
	cmd.AddCommand(newCleanCommand(opts))
	cmd.AddCommand(newDaemonCommand(opts))
	return cmd
}

func bootstrap(ctx context.Context, opts *rootOptions, requireRepo bool) (*runtimeDeps, error) {
	git := project.NewGit()
	repoRoot := ""
	if requireRepo {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root, err := git.FindRepoRoot(ctx, cwd)
		if err != nil {
			return nil, errorsx.Wrap(errorsx.CodeNotGitRepo, "not a git repository", errorsx.ExitUsage, err)
		}
		repoRoot = root
	}
	cfg, err := config.LoadForRepo(ctx, repoRoot, opts.configPath)
	if err != nil {
		return nil, err
	}
	if opts.logLevel != "" {
		cfg.Logging.Level = opts.logLevel
	}
	paths, err := state.Resolve(ctx, cfg.Workspace.StateDir, cfg.Workspace.CacheDir)
	if err != nil {
		return nil, err
	}
	redactor := execx.NewRedactor()
	logger := logging.New(cfg.Logging, os.Stderr, redactor)
	store, err := sqlitestore.Open(ctx, paths.DBPath, logger, nil)
	if err != nil {
		return nil, err
	}
	runner := execx.NewProcessRunner(logger)
	scannerAdapters := []ports.Scanner{scanner.NewGitleaks(runner, nil)}
	for _, engine := range cfg.Scan.Engines {
		if engine == "trufflehog" {
			scannerAdapters = append(scannerAdapters, scanner.NewTruffleHog())
		}
	}
	sandboxAdapter, err := sandbox.New(cfg.Sandbox, runner)
	if err != nil {
		return nil, err
	}
	planner := policy.NewPlanner(git)
	workspaceManager := workspace.NewManager(git)
	registry := agent.NewRegistry(cfg.Agent)
	locks := state.NewFileLockManager(paths, logger)
	runService, err := app.NewRunService(app.RunServiceDeps{
		Store: store, Git: git, Policy: planner, Workspace: workspaceManager, Scanners: scannerAdapters,
		Sandbox: sandboxAdapter, Agents: registry, Locks: locks, Paths: paths, Redactor: redactor, Logger: logger,
		Clock: ports.SystemClock{},
	})
	if err != nil {
		return nil, err
	}
	applyService, err := app.NewApplyService(store, git, ports.SystemClock{}, locks)
	if err != nil {
		return nil, err
	}
	scanService, err := app.NewScanService(app.ScanServiceDeps{Store: store, Git: git, Policy: planner, Workspace: workspaceManager, Scanners: scannerAdapters, Paths: paths, Clock: ports.SystemClock{}})
	if err != nil {
		return nil, err
	}
	cleanService, err := app.NewCleanService(store, paths, cfg.Retention, ports.SystemClock{})
	if err != nil {
		return nil, err
	}
	return &runtimeDeps{
		cfg: cfg, paths: paths, store: store, git: git, runService: runService,
		scanService:  scanService,
		applyService: applyService,
		cleanService: cleanService,
		doctor:       app.NewDoctorService(paths),
		redactor:     redactor,
		logger:       logger,
	}, nil
}

func ExitCode(err error) int {
	if public, ok := errorsx.IsPublic(err); ok && public.ExitCode != 0 {
		return public.ExitCode
	}
	return errorsx.ExitInternal
}

func closeDeps(deps *runtimeDeps) {
	if deps == nil || deps.store == nil {
		return
	}
	if err := deps.store.Close(); err != nil && deps.logger != nil {
		deps.logger.Error("close sqlite store failed", slog.Any("error", err))
	}
}
