package cmd

import (
	"fmt"

	"pgboundary/internal/pgbouncer"
	"pgboundary/internal/process"

	"github.com/spf13/cobra"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Remove stale pgbouncer connection entries",
	Long: `Scans the pgbouncer config for %include entries whose connection
file is missing or whose backing boundary process is no longer running,
and removes them.`,
	RunE: runCleanup,
	PreRun: func(cmd *cobra.Command, args []string) {
		process.Verbose, _ = cmd.Flags().GetBool("verbose")
	},
}

func runCleanup(cmd *cobra.Command, args []string) error {
	removed, err := pgbouncer.Reconcile(Cfg)
	if err != nil {
		return fmt.Errorf("failed to reconcile pgbouncer config: %w", err)
	}

	if len(removed) == 0 {
		fmt.Println("nothing to clean up")
		return nil
	}

	fmt.Printf("cleaned up %d stale connection(s):\n", len(removed))
	for _, entry := range removed {
		fmt.Printf("  %s (%s)\n", entry.Target, entry.Reason)
	}
	return nil
}
