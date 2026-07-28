package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/spf13/cobra"
)

func newRunCommand(opts *rootOptions) *cobra.Command {
	var task string
	var taskFile string
	var agentArgs []string
	var timeout int
	var network string
	var includeUntracked bool
	var includeDirty bool
	var allowSoft bool
	var command string
	cmd := &cobra.Command{
		Use:   "run <agent>",
		Short: "Run an agent in a shadow workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if (task == "") == (taskFile == "") {
				return errorsx.Wrap(errorsx.CodeValidation, "exactly one of --task or --task-file is required", errorsx.ExitUsage, nil)
			}
			taskText := task
			if taskFile != "" {
				data, err := os.ReadFile(taskFile)
				if err != nil {
					return err
				}
				taskText = string(data)
			}
			taskText = strings.TrimSpace(taskText)
			if taskText == "" {
				return errorsx.Wrap(errorsx.CodeValidation, "task is empty", errorsx.ExitUsage, nil)
			}
			deps, err := bootstrap(cmd.Context(), opts, true)
			if err != nil {
				return err
			}
			defer closeDeps(deps)
			var override *config.AgentAdapterConfig
			if command != "" {
				if args[0] != "generic" {
					return errorsx.Wrap(errorsx.CodeValidation, "--command is only valid for generic", errorsx.ExitUsage, nil)
				}
				override = &config.AgentAdapterConfig{Command: command, Args: []string{}, TaskMode: "stdin"}
			}
			result, err := deps.runService.Run(cmd.Context(), domain.RunRequest{
				Agent: args[0], Task: taskText, AgentArgs: agentArgs, TimeoutSeconds: timeout, Network: network,
				IncludeUntracked: includeUntracked, IncludeDirty: includeDirty, AllowSoftMode: allowSoft, AdapterOverride: override, ConfigPath: opts.configPath,
			})
			if opts.json {
				if err != nil {
					return writePublicErrorAndReturn(cmd.OutOrStdout(), err)
				}
				return writeOutput(cmd.OutOrStdout(), true, result)
			}
			if result.RunID != "" {
				_, printErr := fmt.Fprintf(cmd.OutOrStdout(), "run_id: %s\nstatus: %s\ndiff: agentfence diff %s\n", result.RunID, result.Status, result.RunID)
				if printErr != nil {
					return printErr
				}
			}
			return err
		},
	}
	cmd.Flags().StringVar(&task, "task", "", "task text")
	cmd.Flags().StringVar(&taskFile, "task-file", "", "task file")
	cmd.Flags().StringArrayVar(&agentArgs, "agent-arg", nil, "agent argument")
	cmd.Flags().IntVar(&timeout, "timeout", 3600, "timeout seconds")
	cmd.Flags().StringVar(&network, "network", "", "network mode")
	cmd.Flags().BoolVar(&includeUntracked, "include-untracked", false, "include untracked files")
	cmd.Flags().BoolVar(&includeDirty, "include-dirty", false, "include dirty files")
	cmd.Flags().BoolVar(&allowSoft, "allow-soft-mode", false, "allow soft mode")
	cmd.Flags().StringVar(&command, "command", "", "generic command")
	return cmd
}
