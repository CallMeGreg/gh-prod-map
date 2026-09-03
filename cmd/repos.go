package cmd

import (
	"fmt"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var reposCmd = &cobra.Command{
	Use:   "repos",
	Short: "List the repositories in an organization",
	Long: `List the repositories that belong to a GitHub organization.

This command prefers the GraphQL API (via cli/go-gh) over REST because
GraphQL can return the repository name, visibility, primary language, and
star count in a single request.`,
	Example: `  gh prod-map repos --org github
  gh prod-map repos --org github --limit 50
  gh prod-map repos --org github --hostname github.example.com`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepos()
	},
}

func runRepos() error {
	if org_flag == "" {
		return fmt.Errorf("--org is required for the repos command")
	}

	repos, err := ListOrganizationRepositories(org_flag, hostname_flag, limit_flag)
	if err != nil {
		return err
	}

	if len(repos) == 0 {
		PrintWarning("No repositories found for organization: %s", org_flag)
		return nil
	}

	rows := make([][]string, 0, len(repos))
	for _, repo := range repos {
		rows = append(rows, []string{repo.Name, repo.Visibility, repo.Language, fmt.Sprintf("%d", repo.Stars)})
	}

	pterm.Println()
	PrintInfo("Repositories in organization %q: %d", org_flag, len(repos))
	pterm.Println()
	return RenderTable([]string{"Name", "Visibility", "Language", "Stars"}, rows)
}
