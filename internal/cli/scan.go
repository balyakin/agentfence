package cli

import (
	"github.com/agentfence/agentfence/internal/app"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/spf13/cobra"
)

func newScanCommand(opts *rootOptions) *cobra.Command {
	var includeUntracked bool
	var includeDirty bool
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan the shadow workspace without running an agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := bootstrap(cmd.Context(), opts, true)
			if err != nil {
				return err
			}
			defer closeDeps(deps)
			result, err := deps.scanService.Scan(cmd.Context(), app.ScanOnlyRequest{ConfigPath: opts.configPath, IncludeUntracked: includeUntracked, IncludeDirty: includeDirty})
			if err != nil {
				return err
			}
			if err := writeOutput(cmd.OutOrStdout(), true, result); err != nil {
				return err
			}
			if result.Blocked {
				return errorsx.Wrap(
					errorsx.CodeScanBlocked,
					"scan findings blocked the repository",
					errorsx.ExitSecurityBlocked,
					domain.ErrScanBlocked,
				)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&includeUntracked, "include-untracked", false, "include untracked files")
	cmd.Flags().BoolVar(&includeDirty, "include-dirty", false, "include dirty files")
	return cmd
}
