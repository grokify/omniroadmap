package render

import (
	"fmt"
	"strings"

	"github.com/grokify/prism-roadmap/assessment"
)

// PortfolioMarkdown renders review as a markdown portfolio review document.
// Unlike OpportunityMarkdown, this needs no external evidence map: a
// ReportDataset's Ranking already carries denormalized opportunity Titles
// (via RankInput.Title), and every other fact (distributions, capability
// and objective investment, deltas, overrides) is already aggregated —
// nothing here dereferences a raw evidence citation. A pure function of
// review's data (prism-roadmap PRD: "the report is always a pure function
// of the IR").
func PortfolioMarkdown(review assessment.PortfolioReview) string {
	var b strings.Builder

	b.WriteString("# Portfolio Roadmap Review\n\n")
	fmt.Fprintf(&b, "_Generated %s — policy `%s` v%s_\n\n",
		review.Dataset.GeneratedAt.Format("2006-01-02"), review.Dataset.RankingPolicyID, review.Dataset.RankingPolicyVersion)

	for _, s := range review.PresentAgenda() {
		renderPortfolioSection(&b, review, s)
	}

	appendices := review.PresentAppendices()
	if len(appendices) > 0 {
		b.WriteString("---\n\n## Appendices\n\n")
		for _, a := range appendices {
			renderPortfolioAppendix(&b, review, a)
		}
	}

	return b.String()
}

func renderPortfolioSection(b *strings.Builder, review assessment.PortfolioReview, s assessment.ReportSection) {
	fmt.Fprintf(b, "## %s\n\n", s.Title)

	switch s.ID {
	case "prioritized-roadmap":
		renderRankingTable(b, review.Dataset.Ranking, false)
	case "changes-since-previous":
		renderDeltasSummary(b, review.Dataset.Deltas)
	case "methodology":
		renderMethodologySummary(b, review.Dataset)
	case "portfolio-composition":
		for _, d := range review.Dataset.Distributions {
			renderDistribution(b, d, false)
		}
	case "capability-stack":
		renderInvestmentTable(b, "Capability", capabilityRows(review.Dataset.CapabilityOverlay), 5)
	case "strategic-alignment":
		renderInvestmentTable(b, "Objective", objectiveRows(review.Dataset.ObjectiveInvestment), 5)
	case "key-opportunities":
		renderKeyOpportunities(b, review.Dataset.Ranking)
	case "governance-overrides":
		renderOverrideLog(b, review.Dataset.OverrideLog, false)
	}

	renderNarrative(b, s.Narrative)
}

func renderPortfolioAppendix(b *strings.Builder, review assessment.PortfolioReview, a assessment.ReportSection) {
	fmt.Fprintf(b, "### %s\n\n", a.Title)

	switch a.ID {
	case "complete-ranked-roadmap":
		renderRankingTable(b, review.Dataset.Ranking, true)
	case "distribution-detail":
		for _, d := range review.Dataset.Distributions {
			renderDistribution(b, d, true)
		}
	case "investment-detail":
		renderInvestmentTable(b, "Capability", capabilityRows(review.Dataset.CapabilityOverlay), 0)
		b.WriteString("\n")
		renderInvestmentTable(b, "Objective", objectiveRows(review.Dataset.ObjectiveInvestment), 0)
	case "override-log":
		renderOverrideLog(b, review.Dataset.OverrideLog, true)
	case "methodology-detail":
		renderMethodologyDetail(b, review.Dataset)
	}
}

func renderRankingTable(b *strings.Builder, ranking []assessment.OpportunityRank, includeExcluded bool) {
	b.WriteString("| Rank | Title | MoSCoW | RICE | Ties |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, r := range ranking {
		if r.Excluded != "" {
			if includeExcluded {
				fmt.Fprintf(b, "| — | %s | %s | — | excluded (%s) |\n", r.Title, r.MoSCoW, r.Excluded)
			}
			continue
		}
		ties := "—"
		if len(r.TiedWith) > 0 {
			ties = strings.Join(r.TiedWith, ", ")
		}
		rank := fmt.Sprintf("#%d", r.FinalRank)
		if r.Override != nil {
			rank = fmt.Sprintf("#%d (calc #%d)", r.FinalRank, r.CalculatedRank)
		}
		riceCell := "—"
		if r.RICE.Computable {
			riceCell = fmt.Sprintf("%.4f", r.RICE.Score)
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n", rank, r.Title, r.MoSCoW, riceCell, ties)
	}
	b.WriteString("\n")
}

func renderKeyOpportunities(b *strings.Builder, ranking []assessment.OpportunityRank) {
	count := 0
	for _, r := range ranking {
		if r.Excluded != "" {
			continue
		}
		fmt.Fprintf(b, "- **#%d %s** — %s", r.FinalRank, r.Title, r.MoSCoW)
		if r.RICE.Computable {
			fmt.Fprintf(b, ", RICE %.4f", r.RICE.Score)
		}
		if r.Override != nil {
			fmt.Fprintf(b, " (governance override: %s)", r.Override.Rationale)
		}
		b.WriteString("\n")
		count++
		if count >= 5 {
			break
		}
	}
	b.WriteString("\n")
}

func renderDeltasSummary(b *strings.Builder, deltas *assessment.ReportDeltas) {
	if deltas == nil {
		b.WriteString("_First review — no previous dataset to compare against._\n\n")
		return
	}
	fmt.Fprintf(b, "Since %s:\n\n", deltas.PreviousGeneratedAt.Format("2006-01-02"))
	fmt.Fprintf(b, "- **%d** rank moves\n", len(deltas.RankMoves))
	fmt.Fprintf(b, "- **%d** added, **%d** removed\n", len(deltas.Added), len(deltas.Removed))
	fmt.Fprintf(b, "- **%d** portfolio distribution shifts\n\n", len(deltas.DistributionShifts))

	for _, m := range deltas.RankMoves {
		fmt.Fprintf(b, "- `%s`: #%d → #%d\n", m.AssessmentID, m.PreviousRank, m.CurrentRank)
	}
	if len(deltas.RankMoves) > 0 {
		b.WriteString("\n")
	}
}

func renderMethodologySummary(b *strings.Builder, dataset assessment.ReportDataset) {
	fmt.Fprintf(b, "Ranked by policy `%s` v%s: MoSCoW tier first (Must, Should, Could), then RICE score descending within tier.\n\n",
		dataset.RankingPolicyID, dataset.RankingPolicyVersion)
}

func renderMethodologyDetail(b *strings.Builder, dataset assessment.ReportDataset) {
	renderMethodologySummary(b, dataset)
	b.WriteString("Opportunities resolving to Won't/Not Now, or whose RICE could not be computed (insufficient evidence, an unresolved Impact/Confidence threshold, or a failed effort estimability gate), are excluded from the ranking rather than sorted to the bottom with a fabricated score. Same-tier items within a ±5% RICE band are flagged as ties for human tie-break rather than resolved automatically. Governance overrides move an opportunity's final rank away from its calculated rank only with a recorded rationale and approver — never by reweighting inputs.\n\n")
}

// distributionRow is a shared shape for rendering a DimensionDistribution's
// buckets, used at both summary (portfolio-composition) and detail
// (distribution-detail appendix) verbosity.
func renderDistribution(b *strings.Builder, d assessment.DimensionDistribution, detailed bool) {
	fmt.Fprintf(b, "**%s**", d.DimensionID)
	if d.DimensionVersion != "" {
		fmt.Fprintf(b, " (v%s)", d.DimensionVersion)
	}
	b.WriteString("\n\n")

	if detailed {
		b.WriteString("| Option | % Person-Days | Person-Days | Opportunities |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, bucket := range d.Buckets {
			fmt.Fprintf(b, "| %s | %.0f%% | %.1f | %d |\n", bucket.OptionID, bucket.Fraction*100, bucket.PersonDays, bucket.OpportunityCount)
		}
		if d.UnclassifiedPersonDays > 0 {
			fmt.Fprintf(b, "| _unclassified_ | — | %.1f | — |\n", d.UnclassifiedPersonDays)
		}
	} else {
		for _, bucket := range d.Buckets {
			fmt.Fprintf(b, "- %s: %.0f%%\n", bucket.OptionID, bucket.Fraction*100)
		}
	}
	b.WriteString("\n")
}

// investmentRow is the shared shape behind capabilityRows/objectiveRows —
// CapabilityInvestment and ObjectiveInvestment have identical rendering
// needs (an ID, Person-Days, a fraction, and citing opportunity IDs), only
// differing in which ID field they carry.
type investmentRow struct {
	ID             string
	PersonDays     float64
	Fraction       float64
	OpportunityIDs []string
}

func capabilityRows(overlay []assessment.CapabilityInvestment) []investmentRow {
	rows := make([]investmentRow, len(overlay))
	for i, inv := range overlay {
		rows[i] = investmentRow{ID: inv.CapabilityID, PersonDays: inv.PersonDays, Fraction: inv.Fraction, OpportunityIDs: inv.OpportunityIDs}
	}
	return rows
}

func objectiveRows(overlay []assessment.ObjectiveInvestment) []investmentRow {
	rows := make([]investmentRow, len(overlay))
	for i, inv := range overlay {
		rows[i] = investmentRow{ID: inv.ObjectiveID, PersonDays: inv.PersonDays, Fraction: inv.Fraction, OpportunityIDs: inv.OpportunityIDs}
	}
	return rows
}

// renderInvestmentTable renders rows as a table. limit truncates to the
// top N rows (0 = no limit) — used to keep the summary section compact
// while the appendix shows everything.
func renderInvestmentTable(b *strings.Builder, label string, rows []investmentRow, limit int) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(b, "| %s | %% Person-Days | Person-Days | Opportunities |\n", label)
	b.WriteString("|---|---|---|---|\n")
	for i, r := range rows {
		if limit > 0 && i >= limit {
			fmt.Fprintf(b, "| _...%d more_ | | | |\n", len(rows)-limit)
			break
		}
		fmt.Fprintf(b, "| `%s` | %.0f%% | %.1f | %d |\n", r.ID, r.Fraction*100, r.PersonDays, len(r.OpportunityIDs))
	}
	b.WriteString("\n")
}

func renderOverrideLog(b *strings.Builder, overrides []assessment.RankOverride, detailed bool) {
	if len(overrides) == 0 {
		b.WriteString("_No governance overrides applied this review._\n\n")
		return
	}
	for _, o := range overrides {
		fmt.Fprintf(b, "- `%s` → final rank #%d — %s (approved by %s)\n", o.AssessmentID, o.FinalRank, o.Rationale, o.ApprovedBy)
		if detailed && len(o.EvidenceIDs) > 0 {
			fmt.Fprintf(b, "  Evidence: %s\n", strings.Join(o.EvidenceIDs, ", "))
		}
	}
	b.WriteString("\n")
}
