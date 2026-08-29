// Package compile assembles a portfolio-wide assessment.ReportDataset from
// the persisted assessment corpus — the "assessment compiler"
// (prism-roadmap PRD FR12). This is orchestration, not persistence: it
// composes Store calls with prism-roadmap's pure ranking/aggregation
// functions, then persists exactly one new draft dataset.
package compile

import (
	"context"
	"fmt"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
)

// Store is the persistence surface Compile needs, satisfied by
// *store.DoltStore. Defined here rather than imported from store — this
// package tests against a fake, following this repo's existing pattern of
// consumer-side interfaces (see sync.Store).
type Store interface {
	ListCurrentOpportunityAssessments(ctx context.Context) ([]assessment.OpportunityAssessment, error)
	ListDimensions(ctx context.Context) ([]assessment.DimensionDefinition, error)
	ListRankOverrides(ctx context.Context) ([]assessment.RankOverride, error)
	ListProfileAssignments(ctx context.Context) ([]assessment.ProfileAssignment, error)
	GetLatestReportDataset(ctx context.Context) (*assessment.ReportDataset, error)
	SaveReportDataset(ctx context.Context, id string, dataset assessment.ReportDataset, status string) error
}

// gateRankInput enforces omniroadmap's compass-first, two-phase-gated
// ranking pipeline (RMI-OMNIROADMAP-014): an opportunity's RICE score is
// trusted for ranking only when a COMPASS-RICE assessment is recorded AND
// its profile has been PM-confirmed via a matching ProfileAssignment.
// Unlike prism-roadmap's own assessment.OpportunityAssessment.ToRankInput
// (which falls back to the legacy ladder RICE when Compass is nil, for
// other library consumers), omniroadmap's compile pipeline never falls
// back — the two Reach scales are incompatible, so an opportunity without
// a confirmed COMPASS assessment is uncomputable, not legacy-scored.
func gateRankInput(a assessment.OpportunityAssessment, assignments map[string]assessment.ProfileAssignment) assessment.RankInput {
	in := a.ToRankInput()

	if a.Compass == nil {
		in.RICE = assessment.RICEScoreResult{Reason: "awaiting COMPASS assessment"}
		return in
	}

	profileID := string(a.Compass.ProfileID)
	assigned, ok := assignments[a.Opportunity.SpecID]
	if !ok || assigned.Status != assessment.ProfileAssignmentConfirmed || string(assigned.ProfileID) != profileID {
		in.RICE = assessment.RICEScoreResult{Reason: "COMPASS profile not confirmed by PM", ProfileID: profileID}
		return in
	}

	return in
}

// Compile fetches the current assessment corpus, ranks it (applying any
// persisted governance overrides), aggregates portfolio composition across
// every registered dimension, computes deltas against the previous
// compiled dataset (if any), persists the result as a new draft
// ReportDataset under id, and returns it.
//
// RankCollisions are a hard error: presenting a portfolio review with two
// opportunities both at #3 is worse than refusing to compile until the
// conflicting override is fixed (see assessment.RankCollisions).
func Compile(ctx context.Context, s Store, id string, generatedAt time.Time, policy assessment.RankingPolicy) (assessment.ReportDataset, error) {
	assessments, err := s.ListCurrentOpportunityAssessments(ctx)
	if err != nil {
		return assessment.ReportDataset{}, fmt.Errorf("compile: listing assessments: %w", err)
	}

	profileAssignments, err := s.ListProfileAssignments(ctx)
	if err != nil {
		return assessment.ReportDataset{}, fmt.Errorf("compile: listing profile assignments: %w", err)
	}
	assignmentBySpecID := make(map[string]assessment.ProfileAssignment, len(profileAssignments))
	for _, p := range profileAssignments {
		assignmentBySpecID[p.SpecID] = p
	}

	inputs := make([]assessment.RankInput, len(assessments))
	pointers := make([]*assessment.OpportunityAssessment, len(assessments))
	for i := range assessments {
		inputs[i] = gateRankInput(assessments[i], assignmentBySpecID)
		pointers[i] = &assessments[i]
	}

	calculated := policy.Rank(inputs)

	overrides, err := s.ListRankOverrides(ctx)
	if err != nil {
		return assessment.ReportDataset{}, fmt.Errorf("compile: listing rank overrides: %w", err)
	}
	final := assessment.ApplyOverrides(calculated, overrides)

	if collisions := assessment.RankCollisions(final); len(collisions) > 0 {
		return assessment.ReportDataset{}, fmt.Errorf("compile: rank collisions at position(s) %v — resolve the conflicting override(s) before compiling", collisions)
	}

	dataset := assessment.NewReportDataset(generatedAt, policy, final)

	dimensions, err := s.ListDimensions(ctx)
	if err != nil {
		return assessment.ReportDataset{}, fmt.Errorf("compile: listing dimensions: %w", err)
	}
	seen := make(map[string]bool, len(dimensions))
	for _, d := range dimensions {
		// A dimension ID may have multiple registered versions; the
		// distribution is computed once per dimension ID, from whatever
		// version each citing assessment actually recorded.
		if seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		dataset.Distributions = append(dataset.Distributions, assessment.ComputeDimensionDistribution(d.ID, pointers))
	}
	dataset.CapabilityOverlay = assessment.ComputeCapabilityOverlay(pointers)
	dataset.ObjectiveInvestment = assessment.ComputeObjectiveInvestment(pointers)

	previous, err := s.GetLatestReportDataset(ctx)
	if err != nil {
		return assessment.ReportDataset{}, fmt.Errorf("compile: fetching previous dataset: %w", err)
	}
	if previous != nil {
		deltas := assessment.ComputeDeltas(*previous, dataset)
		dataset.Deltas = &deltas
	}

	if err := s.SaveReportDataset(ctx, id, dataset, "draft"); err != nil {
		return assessment.ReportDataset{}, fmt.Errorf("compile: saving dataset: %w", err)
	}

	return dataset, nil
}
