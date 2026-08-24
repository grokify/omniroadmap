package compile

import (
	"context"
	"testing"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
)

// fakeStore records everything saved and serves canned data, following
// this repo's existing in-memory-fake test convention (see sync_test.go).
type fakeStore struct {
	assessments []assessment.OpportunityAssessment
	dimensions  []assessment.DimensionDefinition
	overrides   []assessment.RankOverride
	previous    *assessment.ReportDataset

	saved   []assessment.ReportDataset
	savedID string
}

func (f *fakeStore) ListCurrentOpportunityAssessments(ctx context.Context) ([]assessment.OpportunityAssessment, error) {
	return f.assessments, nil
}

func (f *fakeStore) ListDimensions(ctx context.Context) ([]assessment.DimensionDefinition, error) {
	return f.dimensions, nil
}

func (f *fakeStore) ListRankOverrides(ctx context.Context) ([]assessment.RankOverride, error) {
	return f.overrides, nil
}

func (f *fakeStore) GetLatestReportDataset(ctx context.Context) (*assessment.ReportDataset, error) {
	return f.previous, nil
}

func (f *fakeStore) SaveReportDataset(ctx context.Context, id string, dataset assessment.ReportDataset, status string) error {
	f.savedID = id
	f.saved = append(f.saved, dataset)
	return nil
}

func withEffort(id string, moscowLevel string) assessment.OpportunityAssessment {
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment(id, assessment.OpportunityRef{SpecID: "OPP-" + id}, "Title "+id, now)
	if moscowLevel != "" {
		a.MoSCoWAnswers = []assessment.ThresholdAnswer{
			{LevelID: moscowLevel, Satisfied: true, EvidenceIDs: []string{"EV-1"}},
		}
	}
	a.RICE = &assessment.RICEAssessment{
		Reach: assessment.Reach{Fraction: 0.5, EvidenceIDs: []string{"EV-2"}},
		ImpactAnswers: []assessment.ThresholdAnswer{
			{LevelID: "high", Satisfied: true, EvidenceIDs: []string{"EV-3"}},
		},
		ConfidenceAnswers: []assessment.ThresholdAnswer{
			{LevelID: "high", Satisfied: true, EvidenceIDs: []string{"EV-4"}},
		},
		Effort: assessment.EffortEstimate{
			Expected: 10,
			Gate: assessment.EstimabilityGate{
				ScopeDefined: true, ImplementationIdentified: true, DependenciesIdentified: true,
				TestingIdentified: true, DeploymentIdentified: true,
			},
		},
	}
	return a
}

func TestCompileBasicRanking(t *testing.T) {
	fs := &fakeStore{
		assessments: []assessment.OpportunityAssessment{
			withEffort("OA-1", "must"),
			withEffort("OA-2", "should"),
		},
	}
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)

	dataset, err := Compile(context.Background(), fs, "run-1", now, assessment.DefaultRankingPolicy())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	if len(dataset.Ranking) != 2 {
		t.Fatalf("Ranking = %+v, want 2 entries", dataset.Ranking)
	}
	if dataset.Ranking[0].AssessmentID != "OA-1" {
		t.Errorf("Ranking[0].AssessmentID = %q, want OA-1 (Must beats Should)", dataset.Ranking[0].AssessmentID)
	}
	if fs.savedID != "run-1" || len(fs.saved) != 1 {
		t.Errorf("expected SaveReportDataset to be called once with id run-1, got savedID=%q saved=%d", fs.savedID, len(fs.saved))
	}
}

func TestCompileAppliesPersistedOverrides(t *testing.T) {
	fs := &fakeStore{
		assessments: []assessment.OpportunityAssessment{
			withEffort("OA-1", "must"),
			withEffort("OA-2", "must"),
		},
		overrides: []assessment.RankOverride{
			{AssessmentID: "OA-2", FinalRank: 1, Rationale: "strategic priority", ApprovedBy: "vp"},
			{AssessmentID: "OA-1", FinalRank: 2, Rationale: "deprioritized", ApprovedBy: "vp"},
		},
	}
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)

	dataset, err := Compile(context.Background(), fs, "run-1", now, assessment.DefaultRankingPolicy())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	if dataset.Ranking[0].AssessmentID != "OA-2" || dataset.Ranking[0].FinalRank != 1 {
		t.Errorf("Ranking[0] = %+v, want OA-2 at final rank 1 (override applied)", dataset.Ranking[0])
	}
	if len(dataset.OverrideLog) != 2 {
		t.Errorf("OverrideLog = %+v, want 2 entries", dataset.OverrideLog)
	}
}

func TestCompileRejectsRankCollisions(t *testing.T) {
	fs := &fakeStore{
		assessments: []assessment.OpportunityAssessment{
			withEffort("OA-1", "must"),
			withEffort("OA-2", "must"),
		},
		overrides: []assessment.RankOverride{
			{AssessmentID: "OA-1", FinalRank: 1, Rationale: "r", ApprovedBy: "a"},
			{AssessmentID: "OA-2", FinalRank: 1, Rationale: "r", ApprovedBy: "a"}, // collision
		},
	}
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)

	_, err := Compile(context.Background(), fs, "run-1", now, assessment.DefaultRankingPolicy())
	if err == nil {
		t.Fatal("expected an error for a rank collision, got nil")
	}
	if len(fs.saved) != 0 {
		t.Error("expected no dataset to be saved when compile fails on a collision")
	}
}

func TestCompileAggregatesDimensionDistributions(t *testing.T) {
	a1 := withEffort("OA-1", "must")
	a1.Dimensions = []assessment.DimensionAssignment{
		{DimensionID: "kano", Category: &assessment.CategorySelection{OptionID: "must_be", Resolved: true}},
	}
	fs := &fakeStore{
		assessments: []assessment.OpportunityAssessment{a1},
		dimensions:  []assessment.DimensionDefinition{*assessment.KanoDimension()},
	}
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)

	dataset, err := Compile(context.Background(), fs, "run-1", now, assessment.DefaultRankingPolicy())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(dataset.Distributions) != 1 || dataset.Distributions[0].DimensionID != "kano" {
		t.Fatalf("Distributions = %+v, want one kano entry", dataset.Distributions)
	}
	if len(dataset.Distributions[0].Buckets) != 1 || dataset.Distributions[0].Buckets[0].OptionID != "must_be" {
		t.Errorf("kano buckets = %+v, want must_be", dataset.Distributions[0].Buckets)
	}
}

func TestCompileComputesDeltasAgainstPrevious(t *testing.T) {
	previous := assessment.ReportDataset{
		GeneratedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Ranking: []assessment.OpportunityRank{
			{RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-1"}, FinalRank: 1},
		},
	}
	fs := &fakeStore{
		assessments: []assessment.OpportunityAssessment{withEffort("OA-1", "must")},
		previous:    &previous,
	}
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)

	dataset, err := Compile(context.Background(), fs, "run-1", now, assessment.DefaultRankingPolicy())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if dataset.Deltas == nil {
		t.Fatal("expected Deltas to be set when a previous dataset exists")
	}
	if !dataset.Deltas.PreviousGeneratedAt.Equal(previous.GeneratedAt) {
		t.Errorf("Deltas.PreviousGeneratedAt = %v, want %v", dataset.Deltas.PreviousGeneratedAt, previous.GeneratedAt)
	}
}

func TestCompileNoPreviousDatasetLeavesDeltasNil(t *testing.T) {
	fs := &fakeStore{assessments: []assessment.OpportunityAssessment{withEffort("OA-1", "must")}}
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)

	dataset, err := Compile(context.Background(), fs, "run-1", now, assessment.DefaultRankingPolicy())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if dataset.Deltas != nil {
		t.Errorf("Deltas = %+v, want nil for a first compile", dataset.Deltas)
	}
}

func TestCompileEmptyCorpus(t *testing.T) {
	fs := &fakeStore{}
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)

	dataset, err := Compile(context.Background(), fs, "run-1", now, assessment.DefaultRankingPolicy())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(dataset.Ranking) != 0 {
		t.Errorf("Ranking = %+v, want empty for an empty corpus", dataset.Ranking)
	}
}
