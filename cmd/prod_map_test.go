package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGraphQLClient is a graphqlDoer that replays canned JSON responses,
// unmarshaling each into the caller's response struct.
type fakeGraphQLClient struct {
	pages []string
	calls int
}

func (f *fakeGraphQLClient) Do(_ string, _ map[string]interface{}, out interface{}) error {
	if f.calls >= len(f.pages) {
		return fmt.Errorf("unexpected GraphQL call %d", f.calls+1)
	}
	body := f.pages[f.calls]
	f.calls++
	return json.Unmarshal([]byte(body), out)
}

// withSignalFlags temporarily overrides the package-level sampling limits and
// restores them when the test finishes.
func withSignalFlags(t *testing.T, prLimit, tagLimit, releaseLimit int) {
	t.Helper()
	origPR, origTag, origRelease := prLimitFlag, tagLimitFlag, releaseLimitFlag
	prLimitFlag, tagLimitFlag, releaseLimitFlag = prLimit, tagLimit, releaseLimit
	t.Cleanup(func() {
		prLimitFlag, tagLimitFlag, releaseLimitFlag = origPR, origTag, origRelease
	})
}

func TestTopBranchFromCounts(t *testing.T) {
	branch, count := topBranchFromCounts(map[string]int{"main": 6, "release": 4})
	if branch != "main" || count != 6 {
		t.Fatalf("expected main/6, got %s/%d", branch, count)
	}

	emptyBranch, emptyCount := topBranchFromCounts(map[string]int{})
	if emptyBranch != "-" || emptyCount != 0 {
		t.Fatalf("expected -/0 for empty counts, got %s/%d", emptyBranch, emptyCount)
	}
}

func TestClassifyProductionPattern(t *testing.T) {
	tests := []struct {
		name   string
		signal repoProductionSignals
		want   string
	}{
		{
			name: "release driven",
			signal: repoProductionSignals{
				TotalReleaseCount: 1,
			},
			want: "release-driven",
		},
		{
			name: "tag driven",
			signal: repoProductionSignals{
				TotalTagCount: 2,
			},
			want: "tag-driven",
		},
		{
			name: "trunk driven",
			signal: repoProductionSignals{
				DefaultBranch: "main",
				TopPRBranch:   "main",
			},
			want: "trunk-driven",
		},
		{
			name: "stabilization",
			signal: repoProductionSignals{
				DefaultBranch: "develop",
				TopPRBranch:   "release",
			},
			want: "stabilization-branch",
		},
		{
			name: "insufficient",
			signal: repoProductionSignals{
				DefaultBranch: "main",
				TopPRBranch:   "-",
			},
			want: "insufficient-signals",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyProductionPattern(tc.signal); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestSignalsFromRepoNodeInlineOnly(t *testing.T) {
	withSignalFlags(t, 10, 5, 5)

	client := &fakeGraphQLClient{}
	node := repoSignalNode{Name: "api"}
	node.DefaultBranchRef = &struct{ Name string }{Name: "main"}
	node.Refs.TotalCount = 2
	node.Refs.Nodes = []struct{ Name string }{{Name: "v1.1.0"}, {Name: "v1.0.0"}}
	node.Releases.TotalCount = 1
	node.Releases.Nodes = []struct {
		Name        string
		TagName     string
		PublishedAt string
	}{{Name: "v1.1.0", TagName: "v1.1.0", PublishedAt: "2026-01-01T00:00:00Z"}}
	node.PullRequests.Nodes = []struct{ BaseRefName string }{
		{BaseRefName: "main"}, {BaseRefName: "main"}, {BaseRefName: "release"},
	}

	signals, err := signalsFromRepoNode(client, "acme", node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.calls != 0 {
		t.Fatalf("expected no follow-up PR calls, got %d", client.calls)
	}
	if signals.SampledPRCount != 3 {
		t.Fatalf("expected 3 sampled PRs, got %d", signals.SampledPRCount)
	}
	if signals.TopPRBranch != "main" || signals.TopPRBranchCount != 2 {
		t.Fatalf("expected top branch main/2, got %s/%d", signals.TopPRBranch, signals.TopPRBranchCount)
	}
	if signals.DefaultBranch != "main" {
		t.Fatalf("expected default branch main, got %s", signals.DefaultBranch)
	}
	if signals.TotalTagCount != 2 || len(signals.RecentTags) != 2 || signals.RecentTags[0] != "v1.1.0" {
		t.Fatalf("unexpected tag signals: %+v", signals)
	}
	if signals.RecentRelease == nil || signals.RecentRelease.TagName != "v1.1.0" {
		t.Fatalf("unexpected recent release: %+v", signals.RecentRelease)
	}
	if signals.ProductionPattern != "release-driven" {
		t.Fatalf("expected release-driven, got %s", signals.ProductionPattern)
	}
}

func TestSignalsFromRepoNodeTopUp(t *testing.T) {
	withSignalFlags(t, 5, 5, 5)

	client := &fakeGraphQLClient{pages: []string{
		`{"Repository":{"PullRequests":{"Nodes":[{"BaseRefName":"release"},{"BaseRefName":"release"},{"BaseRefName":"release"}],"PageInfo":{"HasNextPage":false,"EndCursor":""}}}}`,
	}}

	node := repoSignalNode{Name: "api"}
	node.DefaultBranchRef = &struct{ Name string }{Name: "main"}
	node.PullRequests.Nodes = []struct{ BaseRefName string }{
		{BaseRefName: "main"}, {BaseRefName: "main"},
	}
	node.PullRequests.PageInfo.HasNextPage = true
	node.PullRequests.PageInfo.EndCursor = "c1"

	signals, err := signalsFromRepoNode(client, "acme", node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.calls != 1 {
		t.Fatalf("expected exactly 1 follow-up PR page, got %d", client.calls)
	}
	if signals.SampledPRCount != 5 {
		t.Fatalf("expected 5 sampled PRs (2 inline + 3 top-up), got %d", signals.SampledPRCount)
	}
	if signals.TopPRBranch != "release" || signals.TopPRBranchCount != 3 {
		t.Fatalf("expected top branch release/3, got %s/%d", signals.TopPRBranch, signals.TopPRBranchCount)
	}
}

func TestSignalsFromRepoNodePRSamplingDisabled(t *testing.T) {
	withSignalFlags(t, 0, 5, 5)

	client := &fakeGraphQLClient{}
	node := repoSignalNode{Name: "api"}
	node.DefaultBranchRef = &struct{ Name string }{Name: "main"}
	node.PullRequests.Nodes = []struct{ BaseRefName string }{{BaseRefName: "main"}}
	node.PullRequests.PageInfo.HasNextPage = true
	node.PullRequests.PageInfo.EndCursor = "c1"

	signals, err := signalsFromRepoNode(client, "acme", node)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.calls != 0 {
		t.Fatalf("expected no PR calls when sampling disabled, got %d", client.calls)
	}
	if signals.SampledPRCount != 0 || signals.TopPRBranch != "-" {
		t.Fatalf("expected no PR signals, got count=%d branch=%s", signals.SampledPRCount, signals.TopPRBranch)
	}
}

func TestWriteCSVReport(t *testing.T) {
	tempDir := t.TempDir()
	output := filepath.Join(tempDir, "report.csv")
	findings := []repoProductionSignals{
		{
			Owner:             "acme",
			Repository:        "api",
			DefaultBranch:     "main",
			TopPRBranch:       "main",
			TopPRBranchCount:  5,
			SampledPRCount:    9,
			TotalTagCount:     2,
			RecentTags:        []string{"v1.2.0", "v1.1.0"},
			TotalReleaseCount: 1,
			RecentRelease: &releaseSignal{
				Name:        "v1.2.0",
				TagName:     "v1.2.0",
				PublishedAt: "2026-08-31T00:00:00Z",
			},
			ProductionPattern: "release-driven",
		},
	}

	if err := writeCSVReport(output, findings); err != nil {
		t.Fatalf("writeCSVReport returned error: %v", err)
	}

	content, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("failed to read csv: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "owner,repository,default_branch") {
		t.Fatalf("missing csv header, got: %s", text)
	}
	if !strings.Contains(text, "acme,api,main") {
		t.Fatalf("missing expected row, got: %s", text)
	}
}
