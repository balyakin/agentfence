package cli

import (
	"fmt"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/spf13/cobra"
)

func newStatusCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "List run history",
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
			runs, err := deps.store.ListRuns(cmd.Context(), domain.RunFilter{RepoPath: repoRoot, Limit: 50})
			if err != nil {
				return err
			}
			if opts.json {
				return writeOutput(cmd.OutOrStdout(), true, runs)
			}
			for _, run := range runs {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", run.ID, run.Status, run.TaskRedacted); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
