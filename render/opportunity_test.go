package render

import (
	"strings"
	"testing"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
	"github.com/plexusone/structured-evaluation/claims"
)

func minimalReport(t *testing.T) assessment.OpportunityReport {
	t.Helper()
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Unified Authorization Platform", now)
	return assessment.NewOpportunityReport(now, a, nil)
}

func TestOpportunityMarkdownIncludesTitleAndHeader(t *testing.T) {
	md := OpportunityMarkdown(minimalReport(t), nil)
	if !strings.HasPrefix(md, "# Unified Authorization Platform\n") {
		t.Errorf("expected markdown to start with the title heading, got:\n%s", md)
	}
	if !strings.Contains(md, "OA-1") {
		t.Error("expected assessment ID in header")
	}
}

func TestOpportunityMarkdownOmitsAbsentSections(t *testing.T) {
	md := OpportunityMarkdown(minimalReport(t), nil)
	for _, absentTitle := range []string{"Prioritization Rationale", "Portfolio Context", "Strategic & Capability Alignment"} {
		if strings.Contains(md, absentTitle) {
			t.Errorf("expected absent section %q to be omitted from a minimal report, but it appeared", absentTitle)
		}
	}
	// Always-present sections must appear.
	for _, presentTitle := range []string{"Recommendation Summary", "Opportunity Definition", "Risks, Assumptions & Decision"} {
		if !strings.Contains(md, presentTitle) {
			t.Errorf("expected always-present section %q to appear", presentTitle)
		}
	}
}

func TestOpportunityMarkdownOmitsAppendicesWhenNonePresent(t *testing.T) {
	md := OpportunityMarkdown(minimalReport(t), nil)
	if strings.Contains(md, "## Appendices") {
		t.Error("expected no Appendices heading when no appendix is present")
	}
}

func TestOpportunityMarkdownRendersMoSCoWAnswerWithEvidence(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	a.MoSCoWAnswers = []assessment.ThresholdAnswer{
		{LevelID: "must", Satisfied: true, CriterionMet: "KTLO", Rationale: "legacy dependency EOL", EvidenceIDs: []string{"EV-1"}},
	}
	report := assessment.NewOpportunityReport(now, a, nil)

	ev := *assessment.NewEvidence("EV-1", "Vendor dependency reaches EOL in Q4").
		WithSource("https://example.com/eol-notice", assessment.EvidenceSystemOther, claims.ExternalVendorAdvisory, claims.ReliabilityAuthoritative).
		WithExcerpt("Support ends 2026-12-31").
		WithSensitivity(assessment.SensitivityPublic)

	md := OpportunityMarkdown(report, map[string]assessment.Evidence{"EV-1": ev})

	if !strings.Contains(md, "✓ **must** — KTLO") {
		t.Error("expected the satisfied MoSCoW answer to render with its criterion")
	}
	if !strings.Contains(md, "legacy dependency EOL") {
		t.Error("expected the rationale to render")
	}
	if !strings.Contains(md, "Support ends 2026-12-31") {
		t.Error("expected the public evidence excerpt to render")
	}
	if !strings.Contains(md, "https://example.com/eol-notice") {
		t.Error("expected the evidence source URI to render")
	}
}

func TestOpportunityMarkdownGatesRestrictedEvidence(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	a.MoSCoWAnswers = []assessment.ThresholdAnswer{
		{LevelID: "must", Satisfied: true, EvidenceIDs: []string{"EV-1"}},
	}
	report := assessment.NewOpportunityReport(now, a, nil)

	ev := *assessment.NewEvidence("EV-1", "Customer contract requires this by Q3").
		WithSource("https://example.com/contract", assessment.EvidenceSystemContract, claims.ExternalCommunity, claims.ReliabilityHigh).
		WithExcerpt("Section 4.2: confidential terms").
		WithSensitivity(assessment.SensitivityRestricted)

	md := OpportunityMarkdown(report, map[string]assessment.Evidence{"EV-1": ev})

	if strings.Contains(md, "confidential terms") {
		t.Error("restricted evidence excerpt must NOT appear in rendered output")
	}
	if !strings.Contains(md, "access restricted") {
		t.Error("expected an access-restricted note in place of the excerpt")
	}
}

func TestOpportunityMarkdownHandlesMissingEvidenceRecord(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	a.MoSCoWAnswers = []assessment.ThresholdAnswer{
		{LevelID: "must", Satisfied: true, EvidenceIDs: []string{"EV-MISSING"}},
	}
	report := assessment.NewOpportunityReport(now, a, nil)

	md := OpportunityMarkdown(report, map[string]assessment.Evidence{}) // empty map

	if !strings.Contains(md, "EV-MISSING") || !strings.Contains(md, "not found") {
		t.Errorf("expected a not-found note for missing evidence, got:\n%s", md)
	}
}

func TestOpportunityMarkdownRenderRankAndOverride(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	rank := &assessment.OpportunityRank{
		RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-1", CalculatedRank: 5},
		FinalRank:         1,
		Override:          &assessment.RankOverride{AssessmentID: "OA-1", FinalRank: 1, Rationale: "exec ask", ApprovedBy: "vp-product"},
	}
	report := assessment.NewOpportunityReport(now, a, rank)

	md := OpportunityMarkdown(report, nil)

	if !strings.Contains(md, "#1 (calculated #5, overridden)") {
		t.Errorf("expected overridden rank to render both calculated and final, got:\n%s", md)
	}
	if !strings.Contains(md, "exec ask") || !strings.Contains(md, "vp-product") {
		t.Error("expected the override rationale and approver to render in Risks/Decision")
	}
}

func TestOpportunityMarkdownRenderExcludedRank(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	rank := &assessment.OpportunityRank{
		RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-1", Excluded: assessment.ExclusionWont},
	}
	report := assessment.NewOpportunityReport(now, a, rank)

	md := OpportunityMarkdown(report, nil)
	if !strings.Contains(md, "excluded (wont)") {
		t.Errorf("expected excluded rank state to render, got:\n%s", md)
	}
}

func TestOpportunityMarkdownRendersNarrativeWithProvenance(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	report := assessment.NewOpportunityReport(now, a, nil)

	for i := range report.Sections {
		if report.Sections[i].ID == "recommendation" {
			report.Sections[i].Narrative = &assessment.NarrativeSlot{
				ID:          "recommendation-summary",
				Text:        "This is a top priority given the compliance deadline.",
				DerivedFrom: []string{"rank.finalRank"},
				EvidenceIDs: []string{"EV-1"},
			}
		}
	}

	md := OpportunityMarkdown(report, nil)
	if !strings.Contains(md, "This is a top priority given the compliance deadline.") {
		t.Error("expected narrative text to render")
	}
	if !strings.Contains(md, "derived from: rank.finalRank") || !strings.Contains(md, "evidence: EV-1") {
		t.Error("expected narrative provenance footnote to render")
	}
}

func TestOpportunityMarkdownEvidenceLogDeduplicatesAndSorts(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	a.MoSCoWAnswers = []assessment.ThresholdAnswer{
		{LevelID: "must", Satisfied: true, EvidenceIDs: []string{"EV-2", "EV-1"}},
	}
	a.RICE = &assessment.RICEAssessment{
		Reach: assessment.Reach{Fraction: 0.5, EvidenceIDs: []string{"EV-1"}}, // duplicate citation of EV-1
	}
	report := assessment.NewOpportunityReport(now, a, nil)

	ev1 := *assessment.NewEvidence("EV-1", "claim one").WithSource("https://x", assessment.EvidenceSystemOther, claims.ExternalCommunity, claims.ReliabilityHigh).WithSensitivity(assessment.SensitivityPublic)
	ev2 := *assessment.NewEvidence("EV-2", "claim two").WithSource("https://y", assessment.EvidenceSystemOther, claims.ExternalCommunity, claims.ReliabilityHigh).WithSensitivity(assessment.SensitivityPublic)

	md := OpportunityMarkdown(report, map[string]assessment.Evidence{"EV-1": ev1, "EV-2": ev2})

	if strings.Count(md, "Claim: claim one") != 1 {
		t.Errorf("expected EV-1 to appear exactly once in the evidence log despite two citations, got:\n%s", md)
	}
	idx1 := strings.Index(md, "**EV-1**")
	idx2 := strings.Index(md, "**EV-2**")
	if idx1 == -1 || idx2 == -1 || idx1 > idx2 {
		t.Error("expected evidence log entries sorted by evidence ID (EV-1 before EV-2)")
	}
}
