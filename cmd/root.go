package cmd

import (
	"os"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

// Flags that scope the command to a single repository, organization, or
// enterprise, plus the host every API call targets.
var (
	enterprise_flag string
	org_flag        string
	repo_flag       string
	hostname_flag   string
)

// RootCmd is the base command for the prod-map extension. It scans the selected
// scope and detects production branch/tag/release patterns directly, so there
// are no subcommands to disambiguate.
var RootCmd = &cobra.Command{
	Use:   "prod-map [flags]",
	Short: "Map likely production code patterns across GitHub repositories",
	Example: `  gh prod-map --repo github/docs
  gh prod-map --org github --repo-limit 200 --csv-out prod-map.csv
  gh prod-map --enterprise github --org-limit 0 --repo-limit 0 --ai`,
	// pterm renders errors below; let cobra stay quiet so they are not printed twice.
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProdMap()
	},
}

func _root() error {
	RootCmd.CompletionOptions.DisableDefaultCmd = true

	RootCmd.Flags().StringVarP(&repo_flag, "repo", "r", "", "GitHub repository (owner/name)")
	RootCmd.Flags().StringVarP(&org_flag, "org", "o", "", "GitHub organization login")
	RootCmd.Flags().StringVarP(&enterprise_flag, "enterprise", "e", "", "GitHub Enterprise slug (e.g., github)")
	RootCmd.Flags().StringVarP(&hostname_flag, "hostname", "u", "github.com", "GitHub host (e.g., github.example.com for GitHub Enterprise Server)")

	registerProdMapFlags(RootCmd)

	// A run is scoped to exactly one of these, never more than one.
	RootCmd.MarkFlagsMutuallyExclusive("repo", "org", "enterprise")

	return RootCmd.Execute()
}

// Root executes the root command and reports any top-level error.
func Root() {
	if err := _root(); err != nil {
		pterm.Error.Println(err.Error())
		os.Exit(1)
	}
}
