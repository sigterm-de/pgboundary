package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// These variables are set during build time
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

type GitHubRelease struct {
	TagName     string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
}

func getLatestRelease(owner, repo string) (*GitHubRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status: %s", resp.Status)
	}

	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print detailed version information about pgboundary and its dependencies`,
	Run: func(cmd *cobra.Command, args []string) {
		verbose, _ := cmd.Flags().GetBool("verbose")

		// PgBoundary version
		fmt.Printf("pgboundary:\n")
		fmt.Printf(" %s (commit: %s, built: %s)\n", version, commit, buildDate)
		if verbose {
			if release, err := getLatestRelease("sigterm-de", "pgboundary"); err == nil {
				fmt.Printf(" Latest: %s (%s)\n", release.TagName, release.HTMLURL)
			} else {
				fmt.Printf(" Latest: not found\n")
			}
		}

		// Boundary version
		fmt.Printf("\nBoundary CLI:\n")
		boundaryCmd := exec.Command("boundary", "version")
		if output, err := boundaryCmd.Output(); err == nil {
			ver := strings.TrimSpace(string(output))
			fmt.Printf(" %s\n", ver)
			if verbose {
				if release, err := getLatestRelease("hashicorp", "boundary"); err == nil {
					fmt.Printf(" Latest: %s (%s)\n", release.TagName, release.HTMLURL)
				} else {
					fmt.Printf(" Latest: not found\n")
				}
			}
		} else {
			fmt.Printf(" Current: not found\n")
			if verbose {
				fmt.Printf(" Latest: not available\n")
			}
		}

		// PgBouncer version
		fmt.Printf("\nPgBouncer:\n")
		pgbouncerCmd := exec.Command("pgbouncer", "--version")
		if output, err := pgbouncerCmd.Output(); err == nil {
			ver := strings.TrimSpace(string(output))
			fmt.Printf(" %s\n", ver)
			if verbose {
				if release, err := getLatestRelease("pgbouncer", "pgbouncer"); err == nil {
					fmt.Printf(" Latest: %s (%s)\n", release.TagName, release.HTMLURL)
				} else {
					fmt.Printf(" Latest: not found\n")
				}
			}
		} else {
			fmt.Printf(" Current: not found\n")
			if verbose {
				fmt.Printf(" Latest: not available\n")
			}
		}
	},
}

func init() {
	versionCmd.Flags().BoolP("verbose", "v", false, "show verbose version information including latest available releases")
	rootCmd.AddCommand(versionCmd)
}
