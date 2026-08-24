package render

import (
	"fmt"
	"strings"

	sdmarp "github.com/grokify/structureddocs/marp"

	"github.com/grokify/prism-roadmap/assessment"
)

// PresentationMarp renders review's presentation projection
// (assessment.PortfolioReview.PresentationProjection) as a Marp deck — one
// slide per present agenda section, using the shared
// github.com/grokify/structureddocs/marp front matter/theme conventions
// for consistency with the rest of the grokify ecosystem's Marp output
// (see prism-roadmap's goals/v2mom/render/marp). A pure function of
// review's data: the facts are already computed in review.Dataset; this
// only formats them as slides (ideation doc: "the presentation becomes a
// projection of the same document model").
func PresentationMarp(review assessment.PortfolioReview) (string, error) {
	frontMatter, err := sdmarp.RenderFrontMatter(sdmarp.FrontMatterData{
		Theme:    sdmarp.GetTheme("default"),
		Title:    "Portfolio Roadmap Review",
		Subtitle: review.Dataset.GeneratedAt.Format("2006-01-02"),
		Paginate: true,
	})
	if err != nil {
		return "", fmt.Errorf("render: presentation front matter: %w", err)
	}

	var b strings.Builder
	b.WriteString(frontMatter)
	b.WriteString("\n")

	for i, slide := range review.PresentationProjection() {
		if i > 0 {
			b.WriteString("\n---\n\n")
		}
		fmt.Fprintf(&b, "# %s\n\n", slide.Headline)
		renderSlideBody(&b, review, slide)
		if slide.Narrative != nil && slide.Narrative.Text != "" {
			fmt.Fprintf(&b, "%s\n\n", slide.Narrative.Text)
		}
	}

	return b.String(), nil
}

// renderSlideBody adds a compact, slide-appropriate rendering of the
// underlying data for sections that have deterministic facts to show.
// Narrative-only sections (executive-summary, decision-requested,
// methodology summary aside, recommendations, decisions-open-questions)
// get nothing here beyond the headline and their narrative text.
func renderSlideBody(b *strings.Builder, review assessment.PortfolioReview, slide assessment.PresentationSlide) {
	switch slide.SourceSectionID {
	case "prioritized-roadmap":
		renderRankingHighlights(b, review.Dataset.Ranking, 5)
	case "changes-since-previous":
		renderDeltaHighlights(b, review.Dataset.Deltas)
	case "methodology":
		renderMethodologySummary(b, review.Dataset)
	case "portfolio-composition":
		renderDistributionHighlights(b, review.Dataset.Distributions)
	case "capability-stack":
		renderInvestmentHighlights(b, "Capability", capabilityRows(review.Dataset.CapabilityOverlay))
	case "strategic-alignment":
		renderInvestmentHighlights(b, "Objective", objectiveRows(review.Dataset.ObjectiveInvestment))
	case "key-opportunities":
		renderKeyOpportunities(b, review.Dataset.Ranking)
	case "governance-overrides":
		renderOverrideHighlights(b, review.Dataset.OverrideLog)
	}
}

func renderRankingHighlights(b *strings.Builder, ranking []assessment.OpportunityRank, limit int) {
	count := 0
	for _, r := range ranking {
		if r.Excluded != "" {
			continue
		}
		fmt.Fprintf(b, "- **#%d** %s (%s)\n", r.FinalRank, r.Title, r.MoSCoW)
		count++
		if count >= limit {
			break
		}
	}
	b.WriteString("\n")
}

func renderDeltaHighlights(b *strings.Builder, deltas *assessment.ReportDeltas) {
	if deltas == nil {
		b.WriteString("_First review._\n\n")
		return
	}
	fmt.Fprintf(b, "- %d rank moves\n- %d added, %d removed\n- %d distribution shifts\n\n",
		len(deltas.RankMoves), len(deltas.Added), len(deltas.Removed), len(deltas.DistributionShifts))
}

func renderDistributionHighlights(b *strings.Builder, distributions []assessment.DimensionDistribution) {
	for _, d := range distributions {
		fmt.Fprintf(b, "**%s**\n", d.DimensionID)
		if top := topBucket(d.Buckets); top != nil {
			fmt.Fprintf(b, "- Largest: %s (%.0f%%)\n", top.OptionID, top.Fraction*100)
		}
	}
	b.WriteString("\n")
}

// topBucket returns the highest-fraction bucket, or nil for an empty slice
// — used to surface a one-line highlight per dimension on the slide,
// leaving the full breakdown to the document's distribution-detail
// appendix.
func topBucket(buckets []assessment.DistributionBucket) *assessment.DistributionBucket {
	var top *assessment.DistributionBucket
	for i := range buckets {
		if top == nil || buckets[i].Fraction > top.Fraction {
			top = &buckets[i]
		}
	}
	return top
}

func renderInvestmentHighlights(b *strings.Builder, label string, rows []investmentRow) {
	const shown = 3
	for i, r := range rows {
		if i >= shown {
			fmt.Fprintf(b, "- _...%d more_\n", len(rows)-shown)
			break
		}
		fmt.Fprintf(b, "- **%s** `%s`: %.0f%%\n", label, r.ID, r.Fraction*100)
	}
	b.WriteString("\n")
}

func renderOverrideHighlights(b *strings.Builder, overrides []assessment.RankOverride) {
	if len(overrides) == 0 {
		b.WriteString("_No overrides this review._\n\n")
		return
	}
	const shown = 3
	fmt.Fprintf(b, "**%d governance override(s) applied**\n\n", len(overrides))
	for i, o := range overrides {
		if i >= shown {
			fmt.Fprintf(b, "- _...%d more_\n", len(overrides)-shown)
			break
		}
		fmt.Fprintf(b, "- `%s` → #%d: %s\n", o.AssessmentID, o.FinalRank, o.Rationale)
	}
	b.WriteString("\n")
}
