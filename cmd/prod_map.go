package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
)

var (
	repoLimitFlag    int
	orgLimitFlag     int
	prLimitFlag      int
	tagLimitFlag     int
	releaseLimitFlag int
	csvOutFlag       string
	aiFlag           bool
	aiModelFlag      string
)

type repositoryRef struct {
	Owner string
	Name  string
}

type releaseSignal struct {
	Name        string
	TagName     string
	PublishedAt string
}

type repoProductionSignals struct {
	Owner              string
	Repository         string
	DefaultBranch      string
	TopPRBranch        string
	TopPRBranchCount   int
	SampledPRCount     int
	TotalTagCount      int
	RecentTags         []string
	TotalReleaseCount  int
	RecentRelease      *releaseSignal
	ProductionPattern  string
	TargetBranchCounts map[string]int
}

var prodMapCmd = &cobra.Command{
	Use:   "prod-map",
	Short: "Detect production branch/tag/release patterns",
	Long: `Scans repositories in an organization or enterprise and detects likely
production maintenance patterns using default branches, PR base branches, tags,
and releases.`,
	Example: `  gh prod-map prod-map --org github --repo-limit 200 --csv-out prod-map.csv
  gh prod-map prod-map --enterprise github --org-limit 0 --repo-limit 0 --ai`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runProdMap()
	},
}

func init() {
	prodMapCmd.Flags().IntVar(&repoLimitFlag, "repo-limit", 0, "Maximum repositories to scan (0 means all discovered repos)")
	prodMapCmd.Flags().IntVar(&orgLimitFlag, "org-limit", 0, "Maximum organizations to scan when using --enterprise (0 means all discovered organizations)")
	prodMapCmd.Flags().IntVar(&prLimitFlag, "pr-limit", 200, "Maximum pull requests to sample per repository")
	prodMapCmd.Flags().IntVar(&tagLimitFlag, "tag-limit", 20, "Maximum recent tags to include in report details")
	prodMapCmd.Flags().IntVar(&releaseLimitFlag, "release-limit", 20, "Maximum recent releases to include in report details")
	prodMapCmd.Flags().StringVar(&csvOutFlag, "csv-out", "prod-map-report.csv", "Write a detailed CSV report to this path (set empty string to disable)")
	prodMapCmd.Flags().BoolVar(&aiFlag, "ai", false, "Run optional Copilot SDK analysis to summarize production-pattern themes")
	prodMapCmd.Flags().StringVar(&aiModelFlag, "ai-model", "gpt-5-mini", "Model used for optional Copilot SDK analysis")
}

func runProdMap() error {
	if err := ValidateScope(org_flag, enterprise_flag); err != nil {
		return fmt.Errorf("invalid scope for prod-map command: %w", err)
	}
	if org_flag != "" && enterprise_flag != "" {
		return fmt.Errorf("--org and --enterprise cannot be used together")
	}
	if prLimitFlag < 0 || tagLimitFlag < 0 || releaseLimitFlag < 0 || repoLimitFlag < 0 || orgLimitFlag < 0 {
		return fmt.Errorf("limits must be non-negative")
	}

	repos, err := discoverRepositories()
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		PrintWarning("No repositories discovered for the selected scope")
		return nil
	}

	progress, _ := pterm.DefaultProgressbar.WithTitle("Collecting production signals").WithTotal(len(repos)).Start()
	findings := make([]repoProductionSignals, 0, len(repos))
	for _, repo := range repos {
		signals, collectErr := collectRepositoryProductionSignals(repo)
		if collectErr != nil {
			progress.Stop()
			return fmt.Errorf("collecting production signals for %s/%s: %w", repo.Owner, repo.Name, collectErr)
		}
		findings = append(findings, signals)
		progress.Increment()
	}
	progress.Stop()

	stats := buildSummaryStats(findings)
	if err := renderStats(stats); err != nil {
		return fmt.Errorf("rendering summary stats: %w", err)
	}

	if csvOutFlag != "" {
		if err := writeCSVReport(csvOutFlag, findings); err != nil {
			return fmt.Errorf("writing csv report to %q: %w", csvOutFlag, err)
		}
		PrintSuccess("Wrote CSV report: %s", csvOutFlag)
	}

	if aiFlag {
		analysis, analysisErr := analyzePatternsWithCopilot(findings)
		if analysisErr != nil {
			PrintWarning("Copilot AI analysis unavailable: %v", analysisErr)
			analysis = heuristicThemeSummary(findings)
			PrintInfo("Heuristic pattern themes:\n%s", analysis)
		} else {
			PrintSuccess("Copilot AI theme analysis:")
			pterm.Println(analysis)
		}
	}

	return nil
}

func discoverRepositories() ([]repositoryRef, error) {
	if org_flag != "" {
		limit := repoLimitFlag
		if limit == 0 {
			limit = 1000000
		}
		repos, err := ListOrganizationRepositories(org_flag, hostname_flag, limit)
		if err != nil {
			return nil, fmt.Errorf("listing repositories for organization %q: %w", org_flag, err)
		}
		result := make([]repositoryRef, 0, len(repos))
		for _, repo := range repos {
			result = append(result, repositoryRef{Owner: org_flag, Name: repo.Name})
		}
		return result, nil
	}

	orgLimit := orgLimitFlag
	if orgLimit == 0 {
		orgLimit = 1000000
	}

	orgs, err := ListEnterpriseOrganizations(enterprise_flag, hostname_flag, orgLimit)
	if err != nil {
		return nil, fmt.Errorf("listing organizations for enterprise %q: %w", enterprise_flag, err)
	}

	limit := repoLimitFlag
	remaining := repoLimitFlag
	repositories := make([]repositoryRef, 0)
	for _, org := range orgs {
		fetchLimit := 1000000
		if limit > 0 {
			if remaining <= 0 {
				break
			}
			fetchLimit = remaining
		}

		repos, listErr := ListOrganizationRepositories(org.Login, hostname_flag, fetchLimit)
		if listErr != nil {
			return nil, fmt.Errorf("listing repositories for organization %q: %w", org.Login, listErr)
		}
		for _, repo := range repos {
			repositories = append(repositories, repositoryRef{Owner: org.Login, Name: repo.Name})
			if limit > 0 {
				remaining--
				if remaining <= 0 {
					break
				}
			}
		}
	}

	return repositories, nil
}

func collectRepositoryProductionSignals(repo repositoryRef) (repoProductionSignals, error) {
	client, err := NewGraphQLClient(hostname_flag)
	if err != nil {
		return repoProductionSignals{}, fmt.Errorf("creating GraphQL client: %w", err)
	}

	const metadataQuery = `query($owner: String!, $name: String!, $tagFirst: Int!, $releaseFirst: Int!) {
		repository(owner: $owner, name: $name) {
			defaultBranchRef {
				name
			}
			refs(refPrefix: "refs/tags/", first: $tagFirst, orderBy: {field: TAG_COMMIT_DATE, direction: DESC}) {
				totalCount
				nodes {
					name
				}
			}
			releases(first: $releaseFirst, orderBy: {field: CREATED_AT, direction: DESC}) {
				totalCount
				nodes {
					name
					tagName
					publishedAt
				}
			}
		}
	}`

	var metadata struct {
		Repository struct {
			DefaultBranchRef *struct {
				Name string
			}
			Refs struct {
				TotalCount int
				Nodes      []struct {
					Name string
				}
			}
			Releases struct {
				TotalCount int
				Nodes      []struct {
					Name        string
					TagName     string
					PublishedAt string
				}
			}
		}
	}

	if err := DoGraphQLWithRateLimitRetry(client, hostname_flag, metadataQuery, map[string]interface{}{
		"owner":        repo.Owner,
		"name":         repo.Name,
		"tagFirst":     maxInt(tagLimitFlag, 1),
		"releaseFirst": maxInt(releaseLimitFlag, 1),
	}, &metadata); err != nil {
		return repoProductionSignals{}, fmt.Errorf("fetching repository metadata: %w", err)
	}

	prCounts, sampledPRCount, err := collectPRBaseBranchCounts(client, repo.Owner, repo.Name)
	if err != nil {
		return repoProductionSignals{}, err
	}

	topPRBranch, topCount := topBranchFromCounts(prCounts)

	recentTags := make([]string, 0, len(metadata.Repository.Refs.Nodes))
	for _, tag := range metadata.Repository.Refs.Nodes {
		recentTags = append(recentTags, tag.Name)
	}

	var recentRelease *releaseSignal
	if len(metadata.Repository.Releases.Nodes) > 0 {
		n := metadata.Repository.Releases.Nodes[0]
		recentRelease = &releaseSignal{Name: n.Name, TagName: n.TagName, PublishedAt: n.PublishedAt}
	}

	defaultBranch := "-"
	if metadata.Repository.DefaultBranchRef != nil && metadata.Repository.DefaultBranchRef.Name != "" {
		defaultBranch = metadata.Repository.DefaultBranchRef.Name
	}

	signals := repoProductionSignals{
		Owner:              repo.Owner,
		Repository:         repo.Name,
		DefaultBranch:      defaultBranch,
		TopPRBranch:        topPRBranch,
		TopPRBranchCount:   topCount,
		SampledPRCount:     sampledPRCount,
		TotalTagCount:      metadata.Repository.Refs.TotalCount,
		RecentTags:         recentTags,
		TotalReleaseCount:  metadata.Repository.Releases.TotalCount,
		RecentRelease:      recentRelease,
		TargetBranchCounts: prCounts,
	}
	signals.ProductionPattern = classifyProductionPattern(signals)

	return signals, nil
}

func collectPRBaseBranchCounts(client interface {
	Do(string, map[string]interface{}, interface{}) error
}, owner, repo string) (map[string]int, int, error) {
	if prLimitFlag == 0 {
		return map[string]int{}, 0, nil
	}

	counts := make(map[string]int)
	total := 0
	const query = `query($owner: String!, $name: String!, $first: Int!, $endCursor: String) {
		repository(owner: $owner, name: $name) {
			pullRequests(first: $first, after: $endCursor, states: [OPEN, CLOSED, MERGED], orderBy: {field: UPDATED_AT, direction: DESC}) {
				nodes {
					baseRefName
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	}`

	var endCursor *string
	for total < prLimitFlag {
		pageSize := 100
		if remaining := prLimitFlag - total; remaining < pageSize {
			pageSize = remaining
		}

		var response struct {
			Repository struct {
				PullRequests struct {
					Nodes []struct {
						BaseRefName string
					}
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
				}
			}
		}

		if err := DoGraphQLWithRateLimitRetry(client, hostname_flag, query, map[string]interface{}{
			"owner":     owner,
			"name":      repo,
			"first":     pageSize,
			"endCursor": endCursor,
		}, &response); err != nil {
			return nil, 0, fmt.Errorf("fetching pull request base branches: %w", err)
		}

		for _, pr := range response.Repository.PullRequests.Nodes {
			if pr.BaseRefName == "" {
				continue
			}
			counts[pr.BaseRefName]++
			total++
			if total >= prLimitFlag {
				break
			}
		}

		if !response.Repository.PullRequests.PageInfo.HasNextPage {
			break
		}
		cursor := response.Repository.PullRequests.PageInfo.EndCursor
		endCursor = &cursor
	}

	return counts, total, nil
}

type summaryStats struct {
	RepositoryCount            int
	ProductionPatternCounts    map[string]int
	DefaultBranchCounts        map[string]int
	MostTargetedPRBranchCounts map[string]int
}

func buildSummaryStats(findings []repoProductionSignals) summaryStats {
	stats := summaryStats{
		RepositoryCount:            len(findings),
		ProductionPatternCounts:    map[string]int{},
		DefaultBranchCounts:        map[string]int{},
		MostTargetedPRBranchCounts: map[string]int{},
	}
	for _, finding := range findings {
		stats.ProductionPatternCounts[finding.ProductionPattern]++
		stats.DefaultBranchCounts[finding.DefaultBranch]++
		if finding.TopPRBranch != "-" {
			stats.MostTargetedPRBranchCounts[finding.TopPRBranch]++
		}
	}
	return stats
}

func renderStats(stats summaryStats) error {
	PrintSuccess("Scanned repositories: %d", stats.RepositoryCount)
	pterm.Println()

	patternRows := mapToSortedRows(stats.ProductionPatternCounts)
	if err := RenderTable([]string{"Production Pattern", "Repositories"}, patternRows); err != nil {
		return err
	}
	pterm.Println()
	defaultRows := mapToSortedRows(stats.DefaultBranchCounts)
	if err := RenderTable([]string{"Default Branch", "Repositories"}, defaultRows); err != nil {
		return err
	}

	return nil
}

func mapToSortedRows(values map[string]int) [][]string {
	type item struct {
		Key   string
		Count int
	}
	items := make([]item, 0, len(values))
	for key, count := range values {
		items = append(items, item{Key: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{item.Key, strconv.Itoa(item.Count)})
	}
	return rows
}

func writeCSVReport(path string, findings []repoProductionSignals) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
		"owner",
		"repository",
		"default_branch",
		"top_pr_target_branch",
		"top_pr_target_count",
		"sampled_pull_requests",
		"total_tags",
		"recent_tags",
		"total_releases",
		"latest_release_name",
		"latest_release_tag",
		"latest_release_published_at",
		"production_pattern",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, finding := range findings {
		releaseName := ""
		releaseTag := ""
		releasePublishedAt := ""
		if finding.RecentRelease != nil {
			releaseName = finding.RecentRelease.Name
			releaseTag = finding.RecentRelease.TagName
			releasePublishedAt = finding.RecentRelease.PublishedAt
		}

		record := []string{
			finding.Owner,
			finding.Repository,
			finding.DefaultBranch,
			finding.TopPRBranch,
			strconv.Itoa(finding.TopPRBranchCount),
			strconv.Itoa(finding.SampledPRCount),
			strconv.Itoa(finding.TotalTagCount),
			strings.Join(finding.RecentTags, "|"),
			strconv.Itoa(finding.TotalReleaseCount),
			releaseName,
			releaseTag,
			releasePublishedAt,
			finding.ProductionPattern,
		}

		if err := writer.Write(record); err != nil {
			return err
		}
	}

	return writer.Error()
}

func topBranchFromCounts(counts map[string]int) (string, int) {
	if len(counts) == 0 {
		return "-", 0
	}
	keys := make([]string, 0, len(counts))
	for branch := range counts {
		keys = append(keys, branch)
	}
	sort.Strings(keys)

	topBranch := "-"
	topCount := 0
	for _, branch := range keys {
		count := counts[branch]
		if count > topCount {
			topBranch = branch
			topCount = count
		}
	}
	return topBranch, topCount
}

func classifyProductionPattern(signal repoProductionSignals) string {
	if signal.TotalReleaseCount > 0 {
		return "release-driven"
	}
	if signal.TotalTagCount > 0 {
		return "tag-driven"
	}
	if signal.TopPRBranch != "-" && signal.TopPRBranch == signal.DefaultBranch {
		return "trunk-driven"
	}
	if signal.TopPRBranch != "-" && signal.TopPRBranch != signal.DefaultBranch {
		return "stabilization-branch"
	}
	return "insufficient-signals"
}

func analyzePatternsWithCopilot(findings []repoProductionSignals) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client := copilot.NewClient(nil)
	if err := client.Start(ctx); err != nil {
		return "", fmt.Errorf("starting Copilot SDK client: %w", err)
	}
	defer client.Stop()

	session, err := client.CreateSession(ctx, &copilot.SessionConfig{
		ClientName:          "prod-map",
		Model:               aiModelFlag,
		OnPermissionRequest: copilot.PermissionHandler.ApproveAll,
	})
	if err != nil {
		return "", fmt.Errorf("creating Copilot SDK session: %w", err)
	}
	defer session.Disconnect()

	sample := findings
	if len(sample) > 150 {
		sample = sample[:150]
	}

	payload, err := json.Marshal(sample)
	if err != nil {
		return "", fmt.Errorf("marshaling findings for AI analysis: %w", err)
	}

	prompt := "Given this JSON array of repository production signals, identify major themes and bucket similar production patterns. " +
		"Return concise markdown with: 1) top themes, 2) representative branch/tag/release conventions, 3) risk hotspots. JSON:\n" + string(payload)

	response, err := session.SendAndWait(ctx, copilot.MessageOptions{Prompt: prompt})
	if err != nil {
		return "", fmt.Errorf("running Copilot SDK analysis: %w", err)
	}
	if response == nil {
		return "", fmt.Errorf("Copilot SDK analysis returned no response")
	}

	message, ok := response.Data.(*copilot.AssistantMessageData)
	if !ok {
		return "", fmt.Errorf("unexpected Copilot response type: %T", response.Data)
	}

	return strings.TrimSpace(message.Content), nil
}

func heuristicThemeSummary(findings []repoProductionSignals) string {
	stats := buildSummaryStats(findings)
	patterns := mapToSortedRows(stats.ProductionPatternCounts)
	defaultBranches := mapToSortedRows(stats.DefaultBranchCounts)
	lines := []string{"- Theme buckets:"}
	for _, row := range patterns {
		lines = append(lines, fmt.Sprintf("  - %s: %s repos", row[0], row[1]))
	}
	lines = append(lines, "- Common default branches:")
	for i, row := range defaultBranches {
		if i >= 5 {
			break
		}
		lines = append(lines, fmt.Sprintf("  - %s (%s repos)", row[0], row[1]))
	}
	return strings.Join(lines, "\n")
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
