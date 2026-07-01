package cli

import (
	"github.com/spf13/cobra"
)

func newCleanCommand(opts *rootOptions) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean retained run directories",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := bootstrap(cmd.Context(), opts, true)
			if err != nil {
				return err
			}
			defer closeDeps(deps)
			repoRoot, err := deps.git.FindRepoRoot(cmd.Context(), "")
			if err != nil {
				return err
			}
			result, err := deps.cleanService.Clean(cmd.Context(), repoRoot, dryRun)
			if err != nil {
				return err
			}
			return writeOutput(cmd.OutOrStdout(), true, result)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show cleanup candidates")
	return cmd
}
