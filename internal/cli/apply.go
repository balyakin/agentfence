package cli

import (
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/spf13/cobra"
)

func newApplyCommand(opts *rootOptions) *cobra.Command {
	var branch bool
	var branchName string
	var allowDirty bool
	var reject bool
	cmd := &cobra.Command{
		Use:   "apply <run-id|latest>",
		Short: "Apply a reviewed patch",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := bootstrap(cmd.Context(), opts, true)
			if err != nil {
				return err
			}
			defer closeDeps(deps)
			run, err := deps.applyService.Apply(cmd.Context(), domain.ApplyRunRequest{RunID: args[0], CreateBranch: branch, BranchName: branchName, AllowDirtyTree: allowDirty, Reject: reject}, "", opts.configPath)
			if err != nil {
				return err
			}
			return writeOutput(cmd.OutOrStdout(), opts.json, runToMap(run))
		},
	}
	cmd.Flags().BoolVar(&branch, "branch", false, "create branch")
	cmd.Flags().StringVar(&branchName, "branch-name", "", "branch name")
	cmd.Flags().BoolVar(&allowDirty, "allow-dirty-tree", false, "allow dirty tree")
	cmd.Flags().BoolVar(&reject, "reject", false, "write rejects")
	return cmd
}
