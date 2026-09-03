package cmd

import (
	"fmt"

	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var orgsCmd = &cobra.Command{
	Use:   "orgs",
	Short: "List the organizations in an enterprise",
	Long: `List the organizations that belong to a GitHub Enterprise.

Listing the organizations in an enterprise is only supported by the GraphQL
API, so this command always uses GraphQL (via cli/go-gh).`,
	Example: `  gh extension-template orgs --enterprise github
  gh extension-template orgs --enterprise github --limit 100
  gh extension-template orgs --enterprise github --hostname github.example.com`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runOrgs()
	},
}

func runOrgs() error {
	if enterprise_flag == "" {
		return fmt.Errorf("--enterprise is required for the orgs command")
	}

	orgs, err := ListEnterpriseOrganizations(enterprise_flag, hostname_flag, limit_flag)
	if err != nil {
		return err
	}

	if len(orgs) == 0 {
		PrintWarning("No organizations found for enterprise: %s", enterprise_flag)
		return nil
	}

	rows := make([][]string, 0, len(orgs))
	for i, org := range orgs {
		rows = append(rows, []string{fmt.Sprintf("%d", i+1), org.Login, org.Name})
	}

	pterm.Println()
	PrintInfo("Organizations in enterprise %q: %d", enterprise_flag, len(orgs))
	pterm.Println()
	return RenderTable([]string{"#", "Login", "Name"}, rows)
}
