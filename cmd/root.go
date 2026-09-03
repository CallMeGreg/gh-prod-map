package cmd

import (
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// Persistent flags shared across every subcommand.
var (
	enterprise_flag string
	org_flag        string
	hostname_flag   string
	limit_flag      int
)

// RootCmd is the base command for the prod-map extension.
var RootCmd = &cobra.Command{
	Use:   "prod-map <subcommand> [flags]",
	Short: "Map production maintenance patterns across GitHub repositories",
	Long: `prod-map inspects default branches, pull request target branches, tags,
and releases across an organization or enterprise, classifies each repository
into a production-maintenance pattern, and writes a detailed CSV report.

It is built with:
  - spf13/cobra   for subcommands and flags
  - pterm/pterm   for terminal UI (spinners, progress bars, tables)
  - cli/go-gh     for GitHub API calls (GraphQL preferred)`,
	// pterm renders errors below; let cobra stay quiet so they are not printed twice.
	SilenceErrors: true,
	SilenceUsage:  true,
}

func _root() error {
	RootCmd.CompletionOptions.DisableDefaultCmd = true

	RootCmd.PersistentFlags().StringVarP(&enterprise_flag, "enterprise", "e", "", "GitHub Enterprise slug (e.g., github)")
	RootCmd.PersistentFlags().StringVarP(&org_flag, "org", "o", "", "GitHub organization login")
	RootCmd.PersistentFlags().StringVarP(&hostname_flag, "hostname", "u", "github.com", "GitHub host (e.g., github.example.com for GitHub Enterprise Server)")
	RootCmd.PersistentFlags().IntVarP(&limit_flag, "limit", "L", 30, "Maximum number of results to fetch")

	// Commands are scoped to one of these, never both.
	RootCmd.MarkFlagsMutuallyExclusive("enterprise", "org")

	RootCmd.AddCommand(orgsCmd)
	RootCmd.AddCommand(reposCmd)
	RootCmd.AddCommand(prodMapCmd)

	return RootCmd.Execute()
}

// Root executes the root command and reports any top-level error.
func Root() {
	if err := _root(); err != nil {
		pterm.Error.Println(err.Error())
		os.Exit(1)
	}
}
