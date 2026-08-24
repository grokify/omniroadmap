package review

import (
	"context"
	"testing"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
)

// fakeStore records everything saved, following this repo's existing
// in-memory-fake test convention (see sync_test.go, compile_test.go).
type fakeStore struct {
	overrides   map[string]assessment.RankOverride
	assessments map[string]assessment.OpportunityAssessment
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		overrides:   map[string]assessment.RankOverride{},
		assessments: map[string]assessment.OpportunityAssessment{},
	}
}

func (f *fakeStore) SaveRankOverride(ctx context.Context, o assessment.RankOverride) error {
	f.overrides[o.AssessmentID] = o
	return nil
}

func (f *fakeStore) DeleteRankOverride(ctx context.Context, assessmentID string) error {
	delete(f.overrides, assessmentID)
	return nil
}

func (f *fakeStore) SaveOpportunityAssessment(ctx context.Context, a assessment.OpportunityAssessment) error {
	f.assessments[a.ID] = a
	return nil
}

func TestApplyOverride(t *testing.T) {
	fs := newFakeStore()
	edit := Edit{
		Kind: EditOverride,
		Override: &assessment.RankOverride{
			AssessmentID: "OA-1", FinalRank: 1, Rationale: "strategic priority", ApprovedBy: "vp",
		},
	}
	if err := Apply(context.Background(), fs, edit); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, ok := fs.overrides["OA-1"]
	if !ok || got.FinalRank != 1 {
		t.Errorf("overrides[OA-1] = %+v, ok=%v", got, ok)
	}
}

func TestApplyOverrideInvalidRejected(t *testing.T) {
	fs := newFakeStore()
	edit := Edit{
		Kind:     EditOverride,
		Override: &assessment.RankOverride{AssessmentID: "OA-1"}, // missing rationale/approvedBy
	}
	if err := Apply(context.Background(), fs, edit); err == nil {
		t.Error("expected error for an invalid override, got nil")
	}
	if len(fs.overrides) != 0 {
		t.Error("expected nothing persisted for a rejected edit")
	}
}

func TestApplyOverrideMissingRejected(t *testing.T) {
	fs := newFakeStore()
	if err := Apply(context.Background(), fs, Edit{Kind: EditOverride}); err == nil {
		t.Error("expected error when Override is nil")
	}
}

func TestApplyClearOverride(t *testing.T) {
	fs := newFakeStore()
	fs.overrides["OA-1"] = assessment.RankOverride{AssessmentID: "OA-1", FinalRank: 1, Rationale: "r", ApprovedBy: "a"}

	if err := Apply(context.Background(), fs, Edit{Kind: EditClearOverride, AssessmentID: "OA-1"}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, ok := fs.overrides["OA-1"]; ok {
		t.Error("expected override to be cleared")
	}
}

func TestApplyClearOverrideMissingIDRejected(t *testing.T) {
	fs := newFakeStore()
	if err := Apply(context.Background(), fs, Edit{Kind: EditClearOverride}); err == nil {
		t.Error("expected error when AssessmentID is empty")
	}
}

func TestApplyAssessmentEdit(t *testing.T) {
	fs := newFakeStore()
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := assessment.NewOpportunityAssessment("OA-2", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", now)

	if err := Apply(context.Background(), fs, Edit{Kind: EditAssessment, Assessment: a}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got, ok := fs.assessments["OA-2"]
	if !ok || got.Title != "Title" {
		t.Errorf("assessments[OA-2] = %+v, ok=%v", got, ok)
	}
}

func TestApplyAssessmentEditInvalidRejected(t *testing.T) {
	fs := newFakeStore()
	if err := Apply(context.Background(), fs, Edit{Kind: EditAssessment, Assessment: &assessment.OpportunityAssessment{}}); err == nil {
		t.Error("expected error for an invalid assessment")
	}
}

func TestApplyUnknownKindRejected(t *testing.T) {
	fs := newFakeStore()
	if err := Apply(context.Background(), fs, Edit{Kind: "bogus"}); err == nil {
		t.Error("expected error for an unknown edit kind")
	}
}

func TestApplyBatchStopsAtFirstFailure(t *testing.T) {
	fs := newFakeStore()
	edits := []Edit{
		{Kind: EditOverride, Override: &assessment.RankOverride{AssessmentID: "OA-1", FinalRank: 1, Rationale: "r", ApprovedBy: "a"}},
		{Kind: EditOverride, Override: &assessment.RankOverride{AssessmentID: "OA-2"}}, // invalid — missing rationale/approvedBy
		{Kind: EditOverride, Override: &assessment.RankOverride{AssessmentID: "OA-3", FinalRank: 3, Rationale: "r", ApprovedBy: "a"}},
	}
	if err := ApplyBatch(context.Background(), fs, edits); err == nil {
		t.Fatal("expected ApplyBatch to fail on the second edit")
	}
	if _, ok := fs.overrides["OA-1"]; !ok {
		t.Error("expected the first (valid) edit to have been applied before the failure")
	}
	if _, ok := fs.overrides["OA-3"]; ok {
		t.Error("expected the third edit to NOT be applied after the second one failed")
	}
}

func TestApplyBatchAllSucceed(t *testing.T) {
	fs := newFakeStore()
	edits := []Edit{
		{Kind: EditOverride, Override: &assessment.RankOverride{AssessmentID: "OA-1", FinalRank: 1, Rationale: "r", ApprovedBy: "a"}},
		{Kind: EditOverride, Override: &assessment.RankOverride{AssessmentID: "OA-2", FinalRank: 2, Rationale: "r", ApprovedBy: "a"}},
	}
	if err := ApplyBatch(context.Background(), fs, edits); err != nil {
		t.Fatalf("ApplyBatch: %v", err)
	}
	if len(fs.overrides) != 2 {
		t.Errorf("overrides = %+v, want 2 entries", fs.overrides)
	}
}
