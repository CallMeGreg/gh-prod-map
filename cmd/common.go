package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/pterm/pterm"
)

// -----------------------------------------------------------------------------
// API clients (cli/go-gh)
//
// Use these helpers for every GitHub API call. Prefer the GraphQL client: reach
// for REST only when the data you need is not exposed by the GraphQL API.
// -----------------------------------------------------------------------------

// NewGraphQLClient returns a go-gh GraphQL client for the given host. Pass
// "github.com" for GitHub.com or a GitHub Enterprise Server hostname.
func NewGraphQLClient(hostname string) (*api.GraphQLClient, error) {
	return api.NewGraphQLClient(api.ClientOptions{Host: hostname})
}

// NewRESTClient returns a go-gh REST client for the given host. Prefer
// NewGraphQLClient whenever the GraphQL API can return the same data.
func NewRESTClient(hostname string) (*api.RESTClient, error) {
	return api.NewRESTClient(api.ClientOptions{Host: hostname})
}

// DoGraphQLWithRateLimitRetry executes a GraphQL query and retries on primary
// rate-limit errors by waiting until reset time from the rate-limit endpoint.
func DoGraphQLWithRateLimitRetry(client interface {
	Do(string, map[string]interface{}, interface{}) error
}, hostname, query string, variables map[string]interface{}, out interface{}) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := client.Do(query, variables, out); err != nil {
			lastErr = err
			if !isRateLimitError(err) {
				return err
			}
			waitFor, waitErr := waitForPrimaryRateLimitReset(hostname)
			if waitErr != nil {
				return fmt.Errorf("graphql rate limit exceeded and reset wait failed: %w", waitErr)
			}
			PrintWarning("GraphQL rate limit exceeded. Waiting %s before retrying...", waitFor.Round(time.Second))
			time.Sleep(waitFor)
			continue
		}
		return nil
	}
	return fmt.Errorf("graphql rate limit retries exhausted: %w", lastErr)
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "rate limit") || strings.Contains(message, "secondary rate limit")
}

func waitForPrimaryRateLimitReset(hostname string) (time.Duration, error) {
	client, err := NewRESTClient(hostname)
	if err != nil {
		return 0, fmt.Errorf("creating REST client for rate-limit check: %w", err)
	}

	var response struct {
		Resources struct {
			GraphQL struct {
				Remaining int   `json:"remaining"`
				Reset     int64 `json:"reset"`
			} `json:"graphql"`
		} `json:"resources"`
	}

	if err := client.Get("rate_limit", &response); err != nil {
		return 0, fmt.Errorf("querying REST rate-limit endpoint: %w", err)
	}

	if response.Resources.GraphQL.Remaining > 0 {
		return time.Second, nil
	}

	resetAt := time.Unix(response.Resources.GraphQL.Reset, 0)
	waitFor := time.Until(resetAt) + time.Second
	if waitFor < time.Second {
		waitFor = time.Second
	}
	return waitFor, nil
}

// -----------------------------------------------------------------------------
// pterm helpers (pterm/pterm)
// -----------------------------------------------------------------------------

// PrintInfo prints a formatted informational message.
func PrintInfo(format string, args ...interface{}) {
	pterm.Info.Println(fmt.Sprintf(format, args...))
}

// PrintSuccess prints a formatted success message.
func PrintSuccess(format string, args ...interface{}) {
	pterm.Success.Println(fmt.Sprintf(format, args...))
}

// PrintWarning prints a formatted warning message.
func PrintWarning(format string, args ...interface{}) {
	pterm.Warning.Println(fmt.Sprintf(format, args...))
}

// StartSpinner starts and returns a pterm spinner with the given text.
func StartSpinner(text string) (*pterm.SpinnerPrinter, error) {
	return pterm.DefaultSpinner.Start(text)
}

// RenderTable renders a table with a bold header row.
func RenderTable(header []string, rows [][]string) error {
	data := append([][]string{header}, rows...)
	return pterm.DefaultTable.WithHasHeader(true).WithData(data).Render()
}

// -----------------------------------------------------------------------------
// Flag validation
// -----------------------------------------------------------------------------

// ValidateScope ensures exactly one of --org or --enterprise was provided.
func ValidateScope(org, enterprise string) error {
	if org == "" && enterprise == "" {
		return fmt.Errorf("either --org or --enterprise is required")
	}
	return nil
}

// -----------------------------------------------------------------------------
// Example GraphQL queries (cli/go-gh)
// -----------------------------------------------------------------------------

// Organization is a minimal view of a GitHub organization.
type Organization struct {
	Login string
	Name  string
}

// ListEnterpriseOrganizations returns up to limit organizations that belong to
// an enterprise, using the GraphQL API. Listing the organizations in an
// enterprise is only supported by GraphQL, so REST is not an option here.
func ListEnterpriseOrganizations(enterprise, hostname string, limit int) ([]Organization, error) {
	if enterprise == "" {
		return nil, fmt.Errorf("--enterprise is required")
	}
	if limit <= 0 {
		return nil, nil
	}

	client, err := NewGraphQLClient(hostname)
	if err != nil {
		return nil, err
	}

	spinner, _ := StartSpinner(fmt.Sprintf("Fetching organizations for enterprise: %s", enterprise))

	const query = `query($slug: String!, $first: Int!, $endCursor: String) {
		enterprise(slug: $slug) {
			organizations(first: $first, after: $endCursor, orderBy: {field: LOGIN, direction: ASC}) {
				nodes {
					login
					name
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	}`

	const perPage = 100
	var (
		orgs      []Organization
		endCursor *string
	)

	for {
		first := perPage
		if remaining := limit - len(orgs); remaining < first {
			first = remaining
		}

		variables := map[string]interface{}{
			"slug":      enterprise,
			"first":     first,
			"endCursor": endCursor,
		}

		var response struct {
			Enterprise struct {
				Organizations struct {
					Nodes []struct {
						Login string
						Name  string
					}
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
				}
			}
		}

		if err := DoGraphQLWithRateLimitRetry(client, hostname, query, variables, &response); err != nil {
			spinner.Fail(fmt.Sprintf("Failed to fetch organizations for enterprise: %s", enterprise))
			return nil, err
		}

		for _, node := range response.Enterprise.Organizations.Nodes {
			orgs = append(orgs, Organization{Login: node.Login, Name: node.Name})
			if len(orgs) >= limit {
				spinner.Success(fmt.Sprintf("Fetched %d organization(s) from enterprise: %s", len(orgs), enterprise))
				return orgs, nil
			}
		}

		if !response.Enterprise.Organizations.PageInfo.HasNextPage {
			break
		}
		cursor := response.Enterprise.Organizations.PageInfo.EndCursor
		endCursor = &cursor
	}

	spinner.Success(fmt.Sprintf("Fetched %d organization(s) from enterprise: %s", len(orgs), enterprise))
	return orgs, nil
}

// Repository is a minimal view of a GitHub repository.
type Repository struct {
	Name       string
	Visibility string
	Language   string
	Stars      int
}

// ListOrganizationRepositories returns up to limit repositories for an
// organization, using the GraphQL API in preference to REST.
func ListOrganizationRepositories(org, hostname string, limit int) ([]Repository, error) {
	if org == "" {
		return nil, fmt.Errorf("--org is required")
	}
	if limit <= 0 {
		return nil, nil
	}

	client, err := NewGraphQLClient(hostname)
	if err != nil {
		return nil, err
	}

	const query = `query($login: String!, $first: Int!, $endCursor: String) {
		organization(login: $login) {
			repositories(first: $first, after: $endCursor, orderBy: {field: NAME, direction: ASC}) {
				totalCount
				nodes {
					name
					visibility
					stargazerCount
					primaryLanguage {
						name
					}
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	}`

	const perPage = 100
	var (
		repos     []Repository
		endCursor *string
		progress  *pterm.ProgressbarPrinter
	)

	for {
		first := perPage
		if remaining := limit - len(repos); remaining < first {
			first = remaining
		}

		variables := map[string]interface{}{
			"login":     org,
			"first":     first,
			"endCursor": endCursor,
		}

		var response struct {
			Organization struct {
				Repositories struct {
					TotalCount int
					Nodes      []struct {
						Name            string
						Visibility      string
						StargazerCount  int
						PrimaryLanguage struct {
							Name string
						}
					}
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
				}
			}
		}

		if err := DoGraphQLWithRateLimitRetry(client, hostname, query, variables, &response); err != nil {
			if progress != nil {
				progress.Stop()
			}
			return nil, err
		}

		if progress == nil {
			target := response.Organization.Repositories.TotalCount
			if target > limit {
				target = limit
			}
			progress, _ = pterm.DefaultProgressbar.WithTotal(target).WithTitle("Fetching repositories").Start()
		}

		for _, node := range response.Organization.Repositories.Nodes {
			language := node.PrimaryLanguage.Name
			if language == "" {
				language = "-"
			}
			repos = append(repos, Repository{
				Name:       node.Name,
				Visibility: node.Visibility,
				Language:   language,
				Stars:      node.StargazerCount,
			})
			progress.Increment()
			if len(repos) >= limit {
				progress.Stop()
				return repos, nil
			}
		}

		if !response.Organization.Repositories.PageInfo.HasNextPage {
			break
		}
		cursor := response.Organization.Repositories.PageInfo.EndCursor
		endCursor = &cursor
	}

	if progress != nil {
		progress.Stop()
	}
	return repos, nil
}
