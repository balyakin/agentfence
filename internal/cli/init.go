package cli

import (
	"bytes"
	"os"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/spf13/cobra"
)

func newInitCommand(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create .agentfence.yml",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := opts.configPath
			if path == "" {
				path = ".agentfence.yml"
			}
			if _, err := os.Stat(path); err == nil {
				return errorsx.Wrap(errorsx.CodeConfigExists, "config already exists", errorsx.ExitUsage, nil)
			} else if !os.IsNotExist(err) {
				return err
			}
			data, err := config.RenderTemplate()
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				return err
			}
			if _, err := config.LoadBytes(cmd.Context(), data); err != nil {
				return err
			}
			_, err = bytes.NewBufferString("created " + path + "\n").WriteTo(cmd.OutOrStdout())
			return err
		},
	}
}
