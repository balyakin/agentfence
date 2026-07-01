package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/spf13/cobra"
)

func newDiffCommand(opts *rootOptions) *cobra.Command {
	var statOnly bool
	cmd := &cobra.Command{
		Use:   "diff <run-id|latest>",
		Short: "Show a redacted patch",
		Args:  cobra.ExactArgs(1),
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
			var run domain.Run
			if args[0] == "latest" {
				run, err = deps.store.ResolveLatestRun(cmd.Context(), repoRoot)
			} else {
				run, err = deps.store.GetRun(cmd.Context(), args[0])
			}
			if err != nil {
				return err
			}
			data, err := os.ReadFile(run.PatchPath)
			if err != nil {
				return err
			}
			if run.PostScanStatus == "blocked" || run.Status == domain.RunStatusBlockedPost {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "postflight scan blocked raw diff; patch output withheld")
				return err
			}
			output := deps.redactor.RedactString(string(data))
			if statOnly {
				stat, err := gitApplyStat(cmd.Context(), repoRoot, run.PatchPath)
				if err != nil {
					return err
				}
				output = stat
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), output)
			return err
		},
	}
	cmd.Flags().BoolVar(&statOnly, "stat", false, "show stat")
	return cmd
}

func gitApplyStat(ctx context.Context, repoRoot string, patchPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "apply", "--stat", patchPath)
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git apply --stat: %w", err)
	}
	return string(output), nil
}
