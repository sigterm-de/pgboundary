package cmd

import (
	"fmt"

	"pgboundary/internal/boundary"
	"pgboundary/internal/pgbouncer"
	"pgboundary/internal/process"

	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect [target]",
	Short: "Connect to a target",
	Args:  cobra.ExactArgs(1),
	RunE:  runConnect,
	PreRun: func(cmd *cobra.Command, args []string) {
		process.Verbose, _ = cmd.Flags().GetBool("verbose")
	},
}

func runConnect(cmd *cobra.Command, args []string) error {
	target := args[0]
	targetCfg, ok := Cfg.Targets[target]
	if !ok {
		return fmt.Errorf("target %q not found in configuration file", target)
	}

	// Check if target is already connected
	isConnected, err := pgbouncer.IsTargetConnected(Cfg, target)
	if err != nil {
		return fmt.Errorf("failed to check target connection status: %w", err)
	}
	if isConnected {
		fmt.Printf("Warning: target %q is already connected\n", target)
		return nil
	}

	// Get authentication scope
	authScope := targetCfg.Auth
	if authScope == "" {
		authScope = Cfg.Scopes.Auth
	}

	// Get target scope
	targetScope := targetCfg.Scope
	if targetScope == "" {
		targetScope = Cfg.Scopes.Target
	}

	// Start boundary connection
	boundaryConn, err := boundary.StartConnection(targetCfg, authScope, targetScope, Cfg.Auth.Method)
	if err != nil {
		return fmt.Errorf("failed to start boundary connection: %w", err)
	}

	// Update pgbouncer configuration
	if err := pgbouncer.UpdateConfig(Cfg, target, boundaryConn); err != nil {
		return fmt.Errorf("failed to update pgbouncer configuration for target %q: %w", target, err)
	}

	// Reload or start pgbouncer
	if err := pgbouncer.Reload(Cfg); err != nil {
		return fmt.Errorf("failed to reload pgbouncer after adding target %q: %w", target, err)
	}

	return nil
}
