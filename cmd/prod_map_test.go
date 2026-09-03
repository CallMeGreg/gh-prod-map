package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
