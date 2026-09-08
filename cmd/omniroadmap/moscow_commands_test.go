package main

import (
	"testing"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
)

func TestUpsertMoSCoWAnswerAppendsNew(t *testing.T) {
	answers := []assessment.ThresholdAnswer{{LevelID: "must", Satisfied: true}}
	got := upsertMoSCoWAnswer(answers, assessment.ThresholdAnswer{LevelID: "should", Satisfied: true})
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[1].LevelID != "should" {
		t.Errorf("got[1].LevelID = %q, want should", got[1].LevelID)
	}
}

func TestUpsertMoSCoWAnswerReplacesExisting(t *testing.T) {
	answers := []assessment.ThresholdAnswer{
		{LevelID: "must", Satisfied: true, Rationale: "old"},
	}
	got := upsertMoSCoWAnswer(answers, assessment.ThresholdAnswer{LevelID: "must", Satisfied: true, Rationale: "new"})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (should replace, not duplicate)", len(got))
	}
	if got[0].Rationale != "new" {
		t.Errorf("Rationale = %q, want new", got[0].Rationale)
	}
	if answers[0].Rationale != "old" {
		t.Error("upsertMoSCoWAnswer mutated the input slice -- want an independent copy")
	}
}

func TestCarryForwardNextCycleCarriesEverythingExceptIDAndCycle(t *testing.T) {
	now := time.Now()
	prior := assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)
	prior.MoSCoWAnswers = []assessment.ThresholdAnswer{{LevelID: "must", Satisfied: true, EvidenceIDs: []string{"EV-1"}}}
	prior.Dimensions = []assessment.DimensionAssignment{
		{DimensionID: "kano", Category: &assessment.CategorySelection{OptionID: "must_be", Resolved: true}},
	}
	prior.Contributions = []assessment.OKRContribution{{ObjectiveID: "OBJ-1", Strength: assessment.ContributionHigh}}

	next := carryForwardNextCycle(prior, "OA-1-c2", now.Add(time.Hour))

	if next.ID != "OA-1-c2" {
		t.Errorf("ID = %q, want OA-1-c2", next.ID)
	}
	if next.Cycle.Number != 2 || next.Cycle.SupersedesID != "OA-1" {
		t.Errorf("Cycle = %+v, want Number=2 SupersedesID=OA-1", next.Cycle)
	}
	if len(next.MoSCoWAnswers) != 1 || next.MoSCoWAnswers[0].LevelID != "must" {
		t.Errorf("MoSCoWAnswers = %+v, want carried forward", next.MoSCoWAnswers)
	}
	if len(next.Dimensions) != 1 || next.Dimensions[0].DimensionID != "kano" {
		t.Errorf("Dimensions = %+v, want carried forward", next.Dimensions)
	}
	if len(next.Contributions) != 1 || next.Contributions[0].ObjectiveID != "OBJ-1" {
		t.Errorf("Contributions = %+v, want carried forward", next.Contributions)
	}

	// Mutating the copy must not affect the prior cycle.
	next.MoSCoWAnswers[0].LevelID = "should"
	if prior.MoSCoWAnswers[0].LevelID != "must" {
		t.Error("mutating next.MoSCoWAnswers affected prior.MoSCoWAnswers -- want an independent copy")
	}
}
