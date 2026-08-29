// Package render turns prism-roadmap's report contracts
// (assessment.OpportunityReport, PortfolioReview) into markdown output —
// pure functions of their input data (a report/review plus any Evidence
// records they cite). No LLM narrative pass runs here: a NarrativeSlot's
// Text is rendered verbatim if already filled in, and omitted otherwise.
// Every fact rendered — rank, scores, rubric answers, evidence citations —
// comes directly from the input structs, never invented (prism-roadmap
// PRD: "the report is always a pure function of the IR").
package render

import (
	"fmt"
	"sort"
	"strings"
	"time"

	compassrender "github.com/ProductBuildersHQ/compass-rice/render"
	"github.com/grokify/prism-roadmap/assessment"
)

// OpportunityMarkdown renders report as a markdown 6-pager plus appendices.
// evidence supplies the Evidence records report's assessment cites, keyed
// by ID — the caller fetches these (e.g. via the store) before calling, so
// this function stays pure. A citation whose evidence isn't in the map
// renders as "record not found" rather than being silently dropped.
func OpportunityMarkdown(report assessment.OpportunityReport, evidence map[string]assessment.Evidence) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# %s\n\n", report.Assessment.Title)
	fmt.Fprintf(&b, "_Generated %s — Assessment `%s`, cycle %d_\n\n",
		report.GeneratedAt.Format("2006-01-02"), report.Assessment.ID, report.Assessment.Cycle.Number)

	for _, s := range report.PresentSections() {
		renderSection(&b, report, s, evidence)
	}

	appendices := report.PresentAppendices()
	if len(appendices) > 0 {
		b.WriteString("---\n\n## Appendices\n\n")
		for _, a := range appendices {
			renderAppendix(&b, report, a, evidence)
		}
	}

	return b.String()
}

func renderSection(b *strings.Builder, report assessment.OpportunityReport, s assessment.ReportSection, evidence map[string]assessment.Evidence) {
	fmt.Fprintf(b, "## %s\n\n", s.Title)

	switch s.ID {
	case "recommendation":
		renderRecommendation(b, report)
	case "definition":
		renderDefinition(b, report)
	case "prioritization":
		renderPrioritization(b, report, evidence)
	case "portfolio-context":
		renderPortfolioContext(b, report)
	case "strategic-capability":
		renderStrategicCapability(b, report)
	case "risks-decision":
		renderRisksDecision(b, report)
	}

	renderNarrative(b, s.Narrative)
}

func renderRecommendation(b *strings.Builder, report assessment.OpportunityReport) {
	a := report.Assessment
	fmt.Fprintf(b, "- **MoSCoW:** %s\n", a.MoSCoW())

	switch {
	case a.Compass != nil:
		result := assessment.ResolveCompassRICE(a.Compass)
		if result.Computable {
			fmt.Fprintf(b, "- **RICE Score:** %.4f (COMPASS profile: `%s`)\n", result.Score, result.ProfileID)
		} else {
			fmt.Fprintf(b, "- **RICE Score:** not computable — %s\n", result.Reason)
		}
	case a.RICE == nil:
		b.WriteString("- **RICE Score:** not yet assessed\n")
	default:
		result := assessment.ComputeRICE(*a.RICE)
		if result.Computable {
			fmt.Fprintf(b, "- **RICE Score:** %.4f (Impact: %s, Confidence: %s)\n", result.Score, result.Impact, result.Confidence)
		} else {
			fmt.Fprintf(b, "- **RICE Score:** not computable — %s\n", result.Reason)
		}
	}

	switch {
	case report.Rank == nil:
		b.WriteString("- **Opportunity Rank:** not yet ranked\n")
	case report.Rank.Excluded != "":
		fmt.Fprintf(b, "- **Opportunity Rank:** excluded (%s)\n", report.Rank.Excluded)
	case report.Rank.Override != nil:
		fmt.Fprintf(b, "- **Opportunity Rank:** #%d (calculated #%d, overridden)\n", report.Rank.FinalRank, report.Rank.CalculatedRank)
	default:
		fmt.Fprintf(b, "- **Opportunity Rank:** #%d\n", report.Rank.FinalRank)
	}
	b.WriteString("\n")
}

func renderDefinition(b *strings.Builder, report assessment.OpportunityReport) {
	a := report.Assessment
	fmt.Fprintf(b, "- **Opportunity Spec:** `%s`\n", a.Opportunity.SpecID)
	if a.Opportunity.RMIID != "" {
		fmt.Fprintf(b, "- **Roadmap Item:** `%s`\n", a.Opportunity.RMIID)
	}
	fmt.Fprintf(b, "- **Assessment Cycle:** %d (assessed %s)\n", a.Cycle.Number, a.Cycle.AssessedAt.Format("2006-01-02"))
	b.WriteString("\n")
}

func renderPrioritization(b *strings.Builder, report assessment.OpportunityReport, evidence map[string]assessment.Evidence) {
	a := report.Assessment

	if len(a.MoSCoWAnswers) > 0 {
		b.WriteString("**MoSCoW rubric answers:**\n\n")
		for _, ans := range a.MoSCoWAnswers {
			renderAnswer(b, ans.LevelID, ans.Satisfied, ans.CriterionMet, ans.Rationale, ans.EvidenceIDs, evidence)
		}
		b.WriteString("\n")
	}

	if a.Compass != nil {
		renderCompass(b, a)
	}

	if a.RICE == nil {
		return
	}

	fmt.Fprintf(b, "**Reach:** %.0f%%", a.RICE.Reach.Fraction*100)
	if a.RICE.Reach.Rationale != "" {
		fmt.Fprintf(b, " — %s", a.RICE.Reach.Rationale)
	}
	b.WriteString("\n\n")

	if len(a.RICE.ImpactAnswers) > 0 {
		b.WriteString("**Impact rubric answers:**\n\n")
		for _, ans := range a.RICE.ImpactAnswers {
			renderAnswer(b, ans.LevelID, ans.Satisfied, ans.CriterionMet, ans.Rationale, ans.EvidenceIDs, evidence)
		}
		b.WriteString("\n")
	}
	if len(a.RICE.ConfidenceAnswers) > 0 {
		b.WriteString("**Confidence rubric answers:**\n\n")
		for _, ans := range a.RICE.ConfidenceAnswers {
			renderAnswer(b, ans.LevelID, ans.Satisfied, ans.CriterionMet, ans.Rationale, ans.EvidenceIDs, evidence)
		}
		b.WriteString("\n")
	}

	fmt.Fprintf(b, "**Effort:** %.1f person-days (estimability gate: %s)\n\n", a.RICE.Effort.Expected, gateStatus(a.RICE.Effort.Gate))
}

// renderCompass renders a's COMPASS-RICE assessment: computability/review
// status, then the full raw-evidence -> band -> score translation via
// compass-rice's own render.Markdown — never the score alone (compass-rice
// PRD D10), and never reimplemented here, since compass-rice owns how its
// own normalization is explained.
func renderCompass(b *strings.Builder, a assessment.OpportunityAssessment) {
	c := a.Compass
	b.WriteString("**COMPASS-RICE assessment:**\n\n")

	if result := assessment.ResolveCompassRICE(c); !result.Computable {
		fmt.Fprintf(b, "_Not yet computable for ranking — %s._\n\n", result.Reason)
	}

	switch {
	case c.NeedsHumanReview && c.HumanReview == nil:
		b.WriteString("_Flagged for human review — not yet reviewed._\n\n")
	case c.HumanReview != nil:
		fmt.Fprintf(b, "_Reviewed by %s on %s", c.HumanReview.ReviewedBy, c.HumanReview.ReviewedAt.Format("2006-01-02"))
		if c.HumanReview.Note != "" {
			fmt.Fprintf(b, " — %s", c.HumanReview.Note)
		}
		b.WriteString("._\n\n")
	}

	b.WriteString(compassrender.Markdown(c.Normalized))
	b.WriteString("\n")
}

func gateStatus(g assessment.EstimabilityGate) string {
	if g.Passed() {
		return "passed"
	}
	return "NOT passed — " + strings.Join(g.MissingChecks(), "; ")
}

func renderAnswer(b *strings.Builder, levelID string, satisfied bool, criterionMet, rationale string, evidenceIDs []string, evidence map[string]assessment.Evidence) {
	mark := "✗"
	if satisfied {
		mark = "✓"
	}
	fmt.Fprintf(b, "- %s **%s**", mark, levelID)
	if criterionMet != "" {
		fmt.Fprintf(b, " — %s", criterionMet)
	}
	b.WriteString("\n")
	if rationale != "" {
		fmt.Fprintf(b, "  %s\n", rationale)
	}
	for _, evID := range evidenceIDs {
		renderEvidenceCitation(b, evID, evidence)
	}
}

func renderEvidenceCitation(b *strings.Builder, evID string, evidence map[string]assessment.Evidence) {
	e, ok := evidence[evID]
	if !ok {
		fmt.Fprintf(b, "  - `%s` (evidence record not found)\n", evID)
		return
	}
	if excerpt, renderable := e.RenderableExcerpt(); renderable {
		fmt.Fprintf(b, "  - `%s` %q — %s\n", evID, excerpt, e.SourceURI())
		return
	}
	fmt.Fprintf(b, "  - `%s` (%s — access restricted, not rendered)\n", evID, e.Sensitivity)
}

func renderPortfolioContext(b *strings.Builder, report assessment.OpportunityReport) {
	for _, d := range report.Assessment.Dimensions {
		fmt.Fprintf(b, "**%s** (v%s): ", d.DimensionID, d.DimensionVersion)
		switch {
		case d.Category != nil && d.Category.Resolved:
			fmt.Fprintf(b, "%s\n", d.Category.OptionID)
		case d.Category != nil && d.Category.Ambiguous:
			fmt.Fprintf(b, "ambiguous (%s) — needs review\n", strings.Join(d.Category.AmbiguousOptionIDs, ", "))
		case d.Category != nil:
			b.WriteString("unresolved\n")
		default:
			fmt.Fprintf(b, "%s\n", strings.Join(d.Tags, ", "))
		}
	}
	b.WriteString("\n")
}

func renderStrategicCapability(b *strings.Builder, report assessment.OpportunityReport) {
	a := report.Assessment
	if len(a.Contributions) > 0 {
		b.WriteString("**OKR Contributions:**\n\n")
		for _, c := range a.Contributions {
			target := c.ObjectiveID
			if c.KeyResultID != "" {
				target += "/" + c.KeyResultID
			}
			fmt.Fprintf(b, "- `%s` — %s\n", target, c.Strength)
		}
		b.WriteString("\n")
	}
	if len(a.Capabilities) > 0 {
		b.WriteString("**Capabilities:**\n\n")
		for _, cap := range a.Capabilities {
			fmt.Fprintf(b, "- `%s` — %s\n", cap.CapabilityID, cap.Relation)
		}
		b.WriteString("\n")
	}
}

func renderRisksDecision(b *strings.Builder, report assessment.OpportunityReport) {
	if report.Rank != nil && report.Rank.Override != nil {
		o := report.Rank.Override
		fmt.Fprintf(b, "**Governance override applied:** final rank #%d (calculated #%d)\n\n", o.FinalRank, report.Rank.CalculatedRank)
		fmt.Fprintf(b, "> %s\n>\n> — approved by %s\n\n", o.Rationale, o.ApprovedBy)
		return
	}
	b.WriteString("_No governance override applied — this opportunity is at its calculated rank._\n\n")
}

func renderNarrative(b *strings.Builder, n *assessment.NarrativeSlot) {
	if n == nil {
		return
	}
	if n.Text != "" {
		fmt.Fprintf(b, "%s\n\n", n.Text)
	}
	var parts []string
	if len(n.DerivedFrom) > 0 {
		parts = append(parts, "derived from: "+strings.Join(n.DerivedFrom, ", "))
	}
	if len(n.EvidenceIDs) > 0 {
		parts = append(parts, "evidence: "+strings.Join(n.EvidenceIDs, ", "))
	}
	if len(parts) > 0 {
		fmt.Fprintf(b, "<sub>%s</sub>\n\n", strings.Join(parts, " · "))
	}
}

func renderAppendix(b *strings.Builder, report assessment.OpportunityReport, a assessment.ReportSection, evidence map[string]assessment.Evidence) {
	fmt.Fprintf(b, "### %s\n\n", a.Title)
	switch a.ID {
	case "evidence-log":
		renderEvidenceLog(b, report, evidence)
	case "rubric-trace":
		renderRubricTrace(b, report)
	case "provenance":
		renderProvenance(b, report)
	case "related":
		b.WriteString("_No related opportunities computed for this render._\n\n")
	}
}

func renderEvidenceLog(b *strings.Builder, report assessment.OpportunityReport, evidence map[string]assessment.Evidence) {
	refs := report.Assessment.EvidenceReferences()
	seen := make(map[string]bool, len(refs))
	var ids []string
	for _, r := range refs {
		if !seen[r.EvidenceID] {
			seen[r.EvidenceID] = true
			ids = append(ids, r.EvidenceID)
		}
	}
	sort.Strings(ids)

	for _, id := range ids {
		e, ok := evidence[id]
		if !ok {
			fmt.Fprintf(b, "- **%s** — record not found\n\n", id)
			continue
		}
		fmt.Fprintf(b, "- **%s** (%s, %s)", id, e.System, e.Claim.Category)
		if at := e.CapturedAtTime(); at != nil {
			fmt.Fprintf(b, " — captured %s", at.Format("2006-01-02"))
		}
		b.WriteString("\n")
		fmt.Fprintf(b, "  Claim: %s\n", e.Claim.Text)
		if excerpt, renderable := e.RenderableExcerpt(); renderable {
			fmt.Fprintf(b, "  Excerpt: %q\n", excerpt)
			fmt.Fprintf(b, "  Source: %s\n\n", e.SourceURI())
		} else if e.Excerpt() != "" {
			fmt.Fprintf(b, "  Excerpt: (restricted — access via %s)\n\n", e.Sensitivity)
		} else {
			b.WriteString("\n")
		}
	}
}

func renderRubricTrace(b *strings.Builder, report assessment.OpportunityReport) {
	a := report.Assessment
	b.WriteString("| Source | Level/Option | Satisfied | Evidence |\n")
	b.WriteString("|---|---|---|---|\n")
	for _, ans := range a.MoSCoWAnswers {
		fmt.Fprintf(b, "| moscow | %s | %v | %s |\n", ans.LevelID, ans.Satisfied, strings.Join(ans.EvidenceIDs, ", "))
	}
	if a.RICE != nil {
		for _, ans := range a.RICE.ImpactAnswers {
			fmt.Fprintf(b, "| rice.impact | %s | %v | %s |\n", ans.LevelID, ans.Satisfied, strings.Join(ans.EvidenceIDs, ", "))
		}
		for _, ans := range a.RICE.ConfidenceAnswers {
			fmt.Fprintf(b, "| rice.confidence | %s | %v | %s |\n", ans.LevelID, ans.Satisfied, strings.Join(ans.EvidenceIDs, ", "))
		}
	}
	for _, d := range a.Dimensions {
		for _, ans := range d.Answers {
			fmt.Fprintf(b, "| %s | %s | %v | %s |\n", d.DimensionID, ans.OptionID, ans.Answer, strings.Join(ans.EvidenceIDs, ", "))
		}
	}
	b.WriteString("\n")
}

func renderProvenance(b *strings.Builder, report assessment.OpportunityReport) {
	j := report.Assessment.Judge
	if j == nil {
		b.WriteString("_No judge metadata recorded._\n\n")
		return
	}
	fmt.Fprintf(b, "- **Model:** %s\n", j.Model)
	if j.RubricID != "" {
		fmt.Fprintf(b, "- **Rubric:** %s v%s\n", j.RubricID, j.RubricVersion)
	}
	if !j.EvaluatedAt.IsZero() {
		fmt.Fprintf(b, "- **Evaluated At:** %s\n", j.EvaluatedAt.Format(time.RFC3339))
	}
	b.WriteString("\n")
}
