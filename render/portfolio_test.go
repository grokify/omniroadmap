package render

import (
	"strings"
	"testing"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
)

func minimalReview(t *testing.T) assessment.PortfolioReview {
	t.Helper()
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	dataset := assessment.NewReportDataset(now, assessment.DefaultRankingPolicy(), nil)
	return assessment.NewPortfolioReview(dataset)
}

func TestPortfolioMarkdownHeader(t *testing.T) {
	md := PortfolioMarkdown(minimalReview(t))
	if !strings.HasPrefix(md, "# Portfolio Roadmap Review\n") {
		t.Errorf("expected the standard heading, got:\n%s", md)
	}
	if !strings.Contains(md, "moscow-rice-v1") {
		t.Error("expected the ranking policy ID to appear in the header")
	}
}

func TestPortfolioMarkdownOmitsAbsentSections(t *testing.T) {
	md := PortfolioMarkdown(minimalReview(t))
	for _, absent := range []string{"Prioritized Roadmap", "Portfolio Composition", "Capability Stack", "Governance & Overrides"} {
		if strings.Contains(md, absent) {
			t.Errorf("expected absent section %q to be omitted, but it appeared", absent)
		}
	}
	for _, present := range []string{"Executive Summary", "Prioritization Methodology"} {
		if !strings.Contains(md, present) {
			t.Errorf("expected always-present section %q to appear", present)
		}
	}
}

func fullDataset(now time.Time) assessment.ReportDataset {
	policy := assessment.DefaultRankingPolicy()
	ranking := []assessment.OpportunityRank{
		{
			RankedOpportunity: assessment.RankedOpportunity{
				AssessmentID: "OA-1", Title: "Unified Auth Platform", MoSCoW: "must_have",
				RICE: assessment.RICEScoreResult{Score: 0.42, Computable: true}, CalculatedRank: 1,
			},
			FinalRank: 1,
		},
		{
			RankedOpportunity: assessment.RankedOpportunity{
				AssessmentID: "OA-2", Title: "Audit Improvements", MoSCoW: "should_have",
				RICE: assessment.RICEScoreResult{Score: 0.31, Computable: true}, CalculatedRank: 2,
			},
			FinalRank: 3,
			Override:  &assessment.RankOverride{AssessmentID: "OA-2", FinalRank: 3, Rationale: "deprioritized", ApprovedBy: "vp"},
		},
		{
			RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-3", Title: "Deferred Item", Excluded: assessment.ExclusionWont},
		},
	}
	dataset := assessment.NewReportDataset(now, policy, ranking)
	dataset.Distributions = []assessment.DimensionDistribution{
		{
			DimensionID: "kano", DimensionVersion: "1.0",
			Buckets: []assessment.DistributionBucket{
				{OptionID: "must_be", Fraction: 0.6, PersonDays: 30, OpportunityCount: 2},
				{OptionID: "performance", Fraction: 0.4, PersonDays: 20, OpportunityCount: 1},
			},
		},
	}
	dataset.CapabilityOverlay = []assessment.CapabilityInvestment{
		{CapabilityID: "authorization", PersonDays: 30, Fraction: 0.6, OpportunityIDs: []string{"OA-1"}},
	}
	dataset.ObjectiveInvestment = []assessment.ObjectiveInvestment{
		{ObjectiveID: "OBJ-1", PersonDays: 50, Fraction: 1.0, OpportunityIDs: []string{"OA-1", "OA-2"}},
	}
	deltas := assessment.ReportDeltas{
		PreviousGeneratedAt: now.AddDate(0, -3, 0),
		RankMoves:           []assessment.RankMove{{AssessmentID: "OA-2", PreviousRank: 1, CurrentRank: 3}},
		Added:               []string{"OA-1"},
	}
	dataset.Deltas = &deltas
	return dataset
}

func TestPortfolioMarkdownRankingTable(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	review := assessment.NewPortfolioReview(fullDataset(now))

	md := PortfolioMarkdown(review)

	if !strings.Contains(md, "Unified Auth Platform") || !strings.Contains(md, "must_have") {
		t.Error("expected the ranking table to include the top opportunity")
	}
	if !strings.Contains(md, "#3 (calc #2)") {
		t.Errorf("expected the overridden opportunity to show both final and calculated rank, got:\n%s", md)
	}
	// Excluded items must NOT appear in the summary section's table —
	// isolate the summary portion (before Appendices) since the appendix
	// legitimately includes them (see the sibling test).
	summary, _, _ := strings.Cut(md, "## Appendices")
	if strings.Contains(summary, "Deferred Item") {
		t.Error("expected excluded opportunities to be omitted from the summary Prioritized Roadmap section")
	}
}

func TestPortfolioMarkdownCompleteRankedRoadmapAppendixIncludesExcluded(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	review := assessment.NewPortfolioReview(fullDataset(now))

	md := PortfolioMarkdown(review)

	if !strings.Contains(md, "Deferred Item") {
		t.Error("expected the Complete Ranked Roadmap appendix to include excluded opportunities")
	}
	if !strings.Contains(md, "excluded (wont)") {
		t.Error("expected the exclusion reason to render")
	}
}

func TestPortfolioMarkdownDistributionSummaryVsDetail(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	review := assessment.NewPortfolioReview(fullDataset(now))

	md := PortfolioMarkdown(review)

	// Summary section: bullet form.
	if !strings.Contains(md, "- must_be: 60%") {
		t.Errorf("expected summary bullet form for the kano distribution, got:\n%s", md)
	}
	// Detail appendix: table form with Person-Days and counts.
	if !strings.Contains(md, "| must_be | 60% | 30.0 | 2 |") {
		t.Errorf("expected detailed table row in the appendix, got:\n%s", md)
	}
}

func TestPortfolioMarkdownDeltasSummary(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	review := assessment.NewPortfolioReview(fullDataset(now))

	md := PortfolioMarkdown(review)
	if !strings.Contains(md, "1** rank moves") {
		t.Error("expected the rank-move count to render")
	}
	if !strings.Contains(md, "OA-2`: #1 → #3") {
		t.Error("expected the specific rank move to render")
	}
}

func TestPortfolioMarkdownNoDeltasFirstReview(t *testing.T) {
	md := PortfolioMarkdown(minimalReview(t))
	// changes-since-previous is absent (no Deltas) for a first review, so
	// its "first review" copy should not appear at all.
	if strings.Contains(md, "no previous dataset") {
		t.Error("expected the changes-since-previous section to be entirely absent, not rendered with placeholder text")
	}
}

func TestPortfolioMarkdownOverrideLog(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	review := assessment.NewPortfolioReview(fullDataset(now))

	md := PortfolioMarkdown(review)
	if !strings.Contains(md, "OA-2` → final rank #3 — deprioritized (approved by vp)") {
		t.Errorf("expected the override log entry to render, got:\n%s", md)
	}
}

func TestPortfolioMarkdownCapabilityAndObjectiveInvestment(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	review := assessment.NewPortfolioReview(fullDataset(now))

	md := PortfolioMarkdown(review)
	if !strings.Contains(md, "`authorization`") {
		t.Error("expected the capability stack section to render the capability ID")
	}
	if !strings.Contains(md, "`OBJ-1`") {
		t.Error("expected the strategic alignment section to render the objective ID")
	}
}
