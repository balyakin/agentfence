package cli

import (
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/spf13/cobra"
)

func newDoctorCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check dependencies and local state",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := bootstrap(cmd.Context(), opts, false)
			if err != nil {
				return err
			}
			defer closeDeps(deps)
			result := deps.doctor.Check(cmd.Context(), deps.cfg)
			if err := writeOutput(cmd.OutOrStdout(), true, result); err != nil {
				return err
			}
			if result.ExitCode != 0 {
				code := errorsx.CodeInternal
				switch result.ExitCode {
				case errorsx.ExitDependencyMissing:
					code = errorsx.CodeDependencyMissing
				case errorsx.ExitSandboxUnavailable:
					code = errorsx.CodeSandboxUnavailable
				case errorsx.ExitUsage:
					code = errorsx.CodeConfigInvalid
				}
				return errorsx.Wrap(code, "doctor checks failed", result.ExitCode, result.Error())
			}
			return nil
		},
	}
}
