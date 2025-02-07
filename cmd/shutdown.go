package cmd

import (
	"fmt"
	"os"

	"pgboundary/internal/boundary"
	"pgboundary/internal/pgbouncer"
	"pgboundary/internal/process"

	"github.com/spf13/cobra"
)

var shutdownCmd = &cobra.Command{
	Use:   "shutdown [connection]",
	Short: "Shutdown all or specific connections",
	Long: `Shutdown connections to boundary and pgbouncer.
If a connection name is provided, only that connection will be shutdown.
Without arguments, all connections will be shutdown.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runShutdown,
	PreRun: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")
		process.Verbose = verbose
	},
}

func runShutdown(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		// Full shutdown - existing behavior
		if err := pgbouncer.Shutdown(Cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}

		if err := boundary.Shutdown(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}

		if err := pgbouncer.CleanConfig(Cfg); err != nil {
			return fmt.Errorf("failed to clean pgbouncer config: %w", err)
		}

		return nil
	}

	// Selective shutdown
	connection := args[0]
	if err := pgbouncer.ShutdownConnection(Cfg, connection); err != nil {
		return fmt.Errorf("failed to shutdown connection %s: %w", connection, err)
	}

	return nil
}
