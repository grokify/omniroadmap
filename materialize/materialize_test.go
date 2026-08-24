package materialize

import (
	"context"
	"errors"
	"testing"

	"github.com/grokify/prism-roadmap/assessment"
)

// fakeStore records everything written, following this repo's existing
// in-memory-fake test convention.
type fakeStore struct {
	ranks         map[string][2]int // assessmentID -> [calculated, final]
	datasetStatus map[string]string
	failOnRankID  string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		ranks:         map[string][2]int{},
		datasetStatus: map[string]string{},
	}
}

func (f *fakeStore) SetOpportunityRank(ctx context.Context, assessmentID string, calculated, final int) error {
	if assessmentID == f.failOnRankID {
		return errBoom
	}
	f.ranks[assessmentID] = [2]int{calculated, final}
	return nil
}

func (f *fakeStore) SetReportDatasetStatus(ctx context.Context, id, status string) error {
	f.datasetStatus[id] = status
	return nil
}

var errBoom = errors.New("boom")

func TestMaterializeWritesRanksAndMarksFinal(t *testing.T) {
	fs := newFakeStore()
	dataset := assessment.ReportDataset{
		Ranking: []assessment.OpportunityRank{
			{RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-1", CalculatedRank: 1}, FinalRank: 1},
			{RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-2", CalculatedRank: 2}, FinalRank: 3}, // overridden
		},
	}

	if err := Materialize(context.Background(), fs, "run-1", dataset); err != nil {
		t.Fatalf("Materialize: %v", err)
	}

	if got := fs.ranks["OA-1"]; got != [2]int{1, 1} {
		t.Errorf("OA-1 rank = %v, want [1 1]", got)
	}
	if got := fs.ranks["OA-2"]; got != [2]int{2, 3} {
		t.Errorf("OA-2 rank = %v, want [2 3] (calculated vs overridden final)", got)
	}
	if fs.datasetStatus["run-1"] != StatusFinal {
		t.Errorf("dataset status = %q, want %q", fs.datasetStatus["run-1"], StatusFinal)
	}
}

func TestMaterializeSkipsExcluded(t *testing.T) {
	fs := newFakeStore()
	dataset := assessment.ReportDataset{
		Ranking: []assessment.OpportunityRank{
			{RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-1", Excluded: assessment.ExclusionWont}},
		},
	}

	if err := Materialize(context.Background(), fs, "run-1", dataset); err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if _, ok := fs.ranks["OA-1"]; ok {
		t.Error("expected SetOpportunityRank to be skipped for an excluded opportunity")
	}
	// The dataset must still be marked final even when every item was excluded.
	if fs.datasetStatus["run-1"] != StatusFinal {
		t.Errorf("dataset status = %q, want %q", fs.datasetStatus["run-1"], StatusFinal)
	}
}

func TestMaterializePropagatesSetRankFailure(t *testing.T) {
	fs := newFakeStore()
	fs.failOnRankID = "OA-2"
	dataset := assessment.ReportDataset{
		Ranking: []assessment.OpportunityRank{
			{RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-1", CalculatedRank: 1}, FinalRank: 1},
			{RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-2", CalculatedRank: 2}, FinalRank: 2},
		},
	}

	if err := Materialize(context.Background(), fs, "run-1", dataset); err == nil {
		t.Fatal("expected an error when SetOpportunityRank fails")
	}
	if _, ok := fs.ranks["OA-1"]; !ok {
		t.Error("expected OA-1 to have been written before OA-2 failed")
	}
	if _, ok := fs.datasetStatus["run-1"]; ok {
		t.Error("expected dataset status to NOT be set to final when materialization failed partway through")
	}
}

func TestMaterializeEmptyRanking(t *testing.T) {
	fs := newFakeStore()
	if err := Materialize(context.Background(), fs, "run-1", assessment.ReportDataset{}); err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	if fs.datasetStatus["run-1"] != StatusFinal {
		t.Errorf("dataset status = %q, want %q even for an empty ranking", fs.datasetStatus["run-1"], StatusFinal)
	}
}
