package cli

import (
	"strings"

	"github.com/agentfence/agentfence/internal/api"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/spf13/cobra"
)

func newDaemonCommand(opts *rootOptions) *cobra.Command {
	var listen string
	var tokenPath string
	var maxWorkers int
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run local API daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := bootstrap(cmd.Context(), opts, false)
			if err != nil {
				return err
			}
			defer closeDeps(deps)
			var network string
			var address string
			tcpMode := false
			token := ""
			if listen == "" {
				listen = deps.cfg.Daemon.ListenAddress
			}
			if listen == "" || listen == "unix" {
				network = "unix"
				address = deps.cfg.Daemon.SocketPath
			} else {
				network = "tcp"
				address = listen
				tcpMode = true
				if !strings.HasPrefix(listen, "127.0.0.1:") {
					return errorsx.Wrap(errorsx.CodeDaemonNonLoopbackForbidden, "tcp daemon must bind to 127.0.0.1", errorsx.ExitUsage, nil)
				}
				token, err = api.GenerateToken()
				if err != nil {
					return err
				}
				if tokenPath == "" {
					tokenPath = deps.cfg.Daemon.TokenFile
					if tokenPath == "" {
						tokenPath = deps.paths.StateDir + "/daemon.token"
					}
				}
				if err := api.WriteTokenFile(tokenPath, token); err != nil {
					return err
				}
			}
			if maxWorkers <= 0 {
				maxWorkers = deps.cfg.Daemon.MaxWorkers
			}
			queue, err := api.NewRunQueue(cmd.Context(), maxWorkers, deps.runService, deps.store, deps.logger)
			if err != nil {
				return err
			}
			defer func() {
				if err := queue.Stop(); err != nil && deps.logger != nil {
					deps.logger.Debug("stop run queue failed", "error", err)
				}
			}()
			server, err := api.NewServer(api.ServerDeps{
				Store: deps.store, Git: deps.git, RunService: deps.runService, ApplyService: deps.applyService, ScanService: deps.scanService,
				Queue: queue, Paths: deps.paths, Logger: deps.logger, Redactor: deps.redactor, Token: token, TCPMode: tcpMode,
			})
			if err != nil {
				return err
			}
			return server.Serve(cmd.Context(), network, address)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "", "unix or 127.0.0.1:17390")
	cmd.Flags().StringVar(&tokenPath, "token-file", "", "daemon token file")
	cmd.Flags().IntVar(&maxWorkers, "max-workers", 0, "max workers")
	return cmd
}
