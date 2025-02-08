package cmd

import (
	"fmt"

	"pgboundary/internal/pgbouncer"
	"pgboundary/internal/process"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available boundary targets and active pgbouncer connections",
	RunE:  runList,
	PreRun: func(cmd *cobra.Command, args []string) {
		process.Verbose, _ = cmd.Flags().GetBool("verbose")
	},
}

func runList(cmd *cobra.Command, args []string) error {
	// Check PgBouncer status
	if running, pid, err := pgbouncer.CheckStatus(Cfg.PgBouncer.PidFile); err == nil && running {
		if process.Verbose {
			fmt.Printf("PgBouncer is running (pid: %d)\n", pid)
		}
		// Get PgBouncer connections
		connections, err := pgbouncer.GetConnectionDetails(Cfg.PgBouncer.ConfFile)
		if err != nil {
			fmt.Printf("Error getting connections: %v\n", err)
		} else {
			fmt.Println("Active PgBouncer connections:")
			for _, conn := range connections {
				if process.Verbose && conn.BoundaryPid > 0 {
					fmt.Printf("  %s (boundary pid: %d)\n", conn.Name, conn.BoundaryPid)
				} else {
					fmt.Printf("  %s\n", conn.Name)
				}
			}
			fmt.Println()
		}
	} else if process.Verbose {
		fmt.Printf("PgBouncer is not running\n\n")
	}

	// List boundary targets
	fmt.Println("Available boundary targets:")
	for name, target := range Cfg.Targets {
		// Get auth scope, fallback to global config
		authScope := target.Auth
		if authScope == "" {
			authScope = Cfg.Scopes.Auth
		}

		// Get target scope, fallback to global config
		targetScope := target.Scope
		if targetScope == "" {
			targetScope = Cfg.Scopes.Target
		}

		fmt.Printf("  %s:\n", name)
		fmt.Printf("    Host:        %s\n", target.Host)
		fmt.Printf("    Target:      %s\n", target.Target)
		fmt.Printf("    Auth Scope:  %s\n", authScope)
		fmt.Printf("    Target Scope:%s\n", targetScope)
		if target.Database != "" {
			fmt.Printf("    Database:    %s\n", target.Database)
		}
		fmt.Println()
	}
	return nil
}
