package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"pgboundary/config"
	"pgboundary/internal/process"

	"github.com/adrg/xdg"
	"github.com/spf13/cobra"
)

var (
	configFile string
	verbose    bool
	Cfg        *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "pgboundary",
	Short: "pgboundary is a wrapper around Boundary and PgBouncer",
	Long:  `pgboundary is a wrapper around Boundary and PgBouncer to be used in IDE or database tools`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Set verbose flag for all commands
		process.Verbose, _ = cmd.Flags().GetBool("verbose")

		var err error
		if configFile != "" {
			// If -c flag is provided, use that config file
			Cfg, err = config.LoadConfig(configFile)
			if err != nil {
				return fmt.Errorf("failed to load configuration from %s: %w", configFile, err)
			}
			return nil
		}

		// Otherwise, check default locations
		Cfg, err = loadConfigFromDefaultLocations()
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}
		return nil
	},
}

func loadConfigFromDefaultLocations() (*config.Config, error) {
	configName := "pgboundary.ini"
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not determine user home directory: %w", err)
	}

	locations := []string{
		// Current directory
		configName,
		// User's home pgboundary directory
		filepath.Join(homeDir, ".pgboundary", configName),
		// XDG config directory
		filepath.Join(xdg.ConfigHome, "pgboundary", configName),
	}

	var configErr error
	for _, location := range locations {
		conf, err := config.LoadConfig(location)
		if err == nil {
			if process.Verbose {
				fmt.Printf("Using configuration file: %s\n", location)
			}
			return conf, nil
		}
		if configErr == nil {
			configErr = err
		}
	}

	return nil, fmt.Errorf("could not find configuration file in default locations (./pgboundary.ini, ~/.pgboundary/pgboundary.ini, $XDG_CONFIG_HOME/pgboundary/pgboundary.ini): %w", configErr)
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file (default: ./pgboundary.ini, ~/.pgboundary/pgboundary.ini, or $XDG_CONFIG_HOME/pgboundary/pgboundary.ini)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")

	rootCmd.AddCommand(listCmd, connectCmd, shutdownCmd, versionCmd)
}
