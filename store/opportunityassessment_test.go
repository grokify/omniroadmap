package store

import (
	"testing"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
)

func TestProjectAssessmentMinimal(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)

	proj := projectAssessment(a)

	if proj.moscowClass != "wont_have" {
		t.Errorf("moscowClass = %q, want wont_have (a.MoSCoW() floors at WontHave with no answers)", proj.moscowClass)
	}
	if proj.riceComputable {
		t.Error("riceComputable = true, want false when no RICE assessment recorded")
	}
	if proj.riceScore != nil {
		t.Errorf("riceScore = %v, want nil", proj.riceScore)
	}
	if proj.kanoCategory != "" || proj.mihCategory != "" {
		t.Errorf("kanoCategory/mihCategory = %q/%q, want empty", proj.kanoCategory, proj.mihCategory)
	}
}

func TestProjectAssessmentMoSCoW(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	a.MoSCoWAnswers = []assessment.ThresholdAnswer{
		{LevelID: "must", Satisfied: true, EvidenceIDs: []string{"EV-1"}},
	}

	proj := projectAssessment(a)
	if proj.moscowClass != "must_have" {
		t.Errorf("moscowClass = %q, want must_have", proj.moscowClass)
	}
}

func TestProjectAssessmentRICEComputable(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	a.RICE = &assessment.RICEAssessment{
		Reach: assessment.Reach{Fraction: 0.5, EvidenceIDs: []string{"EV-1"}},
		ImpactAnswers: []assessment.ThresholdAnswer{
			{LevelID: "high", Satisfied: true, EvidenceIDs: []string{"EV-2"}},
		},
		ConfidenceAnswers: []assessment.ThresholdAnswer{
			{LevelID: "high", Satisfied: true, EvidenceIDs: []string{"EV-3"}},
		},
		Effort: assessment.EffortEstimate{
			Expected: 10,
			Gate: assessment.EstimabilityGate{
				ScopeDefined: true, ImplementationIdentified: true, DependenciesIdentified: true,
				TestingIdentified: true, DeploymentIdentified: true,
			},
		},
	}

	proj := projectAssessment(a)
	if !proj.riceComputable {
		t.Fatal("riceComputable = false, want true for a fully evidenced RICE assessment")
	}
	if proj.riceScore == nil {
		t.Fatal("riceScore = nil, want a computed score")
	}
	// (0.5 * 2.0 * 1.0) / 10 = 0.10
	if *proj.riceScore != 0.10 {
		t.Errorf("riceScore = %v, want 0.10", *proj.riceScore)
	}
}

func TestProjectAssessmentRICEUncomputable(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	a.RICE = &assessment.RICEAssessment{
		Reach: assessment.Reach{Fraction: 0.5, EvidenceIDs: []string{"EV-1"}},
		// No ImpactAnswers — RICE cannot be computed.
	}

	proj := projectAssessment(a)
	if proj.riceComputable {
		t.Error("riceComputable = true, want false when Impact is unresolved")
	}
	if proj.riceScore != nil {
		t.Errorf("riceScore = %v, want nil for an uncomputable result (never fabricated)", proj.riceScore)
	}
}

func TestProjectAssessmentKanoAndMIH(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	a.Dimensions = []assessment.DimensionAssignment{
		{DimensionID: "kano", Category: &assessment.CategorySelection{OptionID: "performance", Resolved: true}},
		{DimensionID: "market-investment-horizon", Category: &assessment.CategorySelection{OptionID: "sam_som", Resolved: true}},
	}

	proj := projectAssessment(a)
	if proj.kanoCategory != "performance" {
		t.Errorf("kanoCategory = %q, want performance", proj.kanoCategory)
	}
	if proj.mihCategory != "sam_som" {
		t.Errorf("mihCategory = %q, want sam_som", proj.mihCategory)
	}
}

func TestProjectAssessmentUnresolvedDimensionLeavesCategoryEmpty(t *testing.T) {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	a.Dimensions = []assessment.DimensionAssignment{
		{DimensionID: "kano", Category: &assessment.CategorySelection{}}, // unresolved
	}

	proj := projectAssessment(a)
	if proj.kanoCategory != "" {
		t.Errorf("kanoCategory = %q, want empty for an unresolved category selection", proj.kanoCategory)
	}
}
