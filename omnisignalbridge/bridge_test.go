package omnisignalbridge

import (
	"context"
	"testing"

	"github.com/grokify/prism-roadmap/assessment"
	"github.com/grokify/prism-roadmap/canvas"
	"github.com/plexusone/signal-spec/pkg/rootcause"
)

type fakeStore struct {
	specs       map[string]canvas.OpportunitySpec
	assessments map[string]assessment.OpportunityAssessment
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		specs:       make(map[string]canvas.OpportunitySpec),
		assessments: make(map[string]assessment.OpportunityAssessment),
	}
}

func (f *fakeStore) SaveOpportunitySpec(ctx context.Context, spec canvas.OpportunitySpec) error {
	f.specs[spec.Metadata.ID] = spec
	return nil
}

func (f *fakeStore) SaveOpportunityAssessment(ctx context.Context, a assessment.OpportunityAssessment) error {
	f.assessments[a.ID] = a
	return nil
}

func TestBridge_SynthesizeRootCause(t *testing.T) {
	client := &fakeLLMClient{response: fakeLLMResponse}
	rootSynth := NewRootCauseSynthesizer(client, RootCauseSynthesizerConfig{})
	store := newFakeStore()
	bridge := NewBridge(rootSynth, nil, store)

	rc := testRootCause()
	if err := bridge.SynthesizeRootCause(context.Background(), rc); err != nil {
		t.Fatalf("SynthesizeRootCause: %v", err)
	}

	wantSpecID := "omnisignal:root_cause:rc-1"
	spec, ok := store.specs[wantSpecID]
	if !ok {
		t.Fatalf("no spec saved with ID %s; saved: %v", wantSpecID, store.specs)
	}
	if spec.UsersAndProblem.ProblemStatement != rc.Title {
		t.Errorf("ProblemStatement = %q, want RootCause.Title %q", spec.UsersAndProblem.ProblemStatement, rc.Title)
	}

	wantAssessmentID := "OA-" + wantSpecID
	a, ok := store.assessments[wantAssessmentID]
	if !ok {
		t.Fatalf("no assessment saved with ID %s; saved: %v", wantAssessmentID, store.assessments)
	}
	if a.RICE.Reach.Fraction != 1.0 {
		t.Errorf("Reach.Fraction = %v, want 1.0 (RootCause had 2 affected customers)", a.RICE.Reach.Fraction)
	}
}

func TestBridge_SynthesizeRootCause_NotEligible(t *testing.T) {
	client := &fakeLLMClient{response: fakeLLMResponse}
	rootSynth := NewRootCauseSynthesizer(client, RootCauseSynthesizerConfig{})
	store := newFakeStore()
	bridge := NewBridge(rootSynth, nil, store)

	rc := testRootCause()
	rc.Status = rootcause.StatusArchived

	if err := bridge.SynthesizeRootCause(context.Background(), rc); err == nil {
		t.Fatal("expected error for an archived (ineligible) root cause")
	}
	if len(store.specs) != 0 {
		t.Error("no spec should be saved for an ineligible root cause")
	}
}

func TestBridge_SynthesizeIdea(t *testing.T) {
	ideaSynth := NewIdeaSynthesizer()
	store := newFakeStore()
	bridge := NewBridge(nil, ideaSynth, store)

	sig := testIdeaSignal()
	if err := bridge.SynthesizeIdea(context.Background(), sig); err != nil {
		t.Fatalf("SynthesizeIdea: %v", err)
	}

	wantSpecID := "omnisignal:idea:" + sig.ID
	spec, ok := store.specs[wantSpecID]
	if !ok {
		t.Fatalf("no spec saved with ID %s; saved: %v", wantSpecID, store.specs)
	}
	if spec.SolutionIdeas.Ideas[0].Description != sig.Description {
		t.Errorf("solution description = %q, want idea description %q", spec.SolutionIdeas.Ideas[0].Description, sig.Description)
	}

	a, ok := store.assessments["OA-"+wantSpecID]
	if !ok {
		t.Fatal("no assessment saved for idea signal")
	}
	if a.RICE.Reach.Fraction != 1.0 {
		t.Errorf("Reach.Fraction = %v, want 1.0 (idea had 3 distinct voter orgs)", a.RICE.Reach.Fraction)
	}
}

func TestBridge_SynthesizeIdea_DoesNotQualify(t *testing.T) {
	ideaSynth := NewIdeaSynthesizer()
	store := newFakeStore()
	bridge := NewBridge(nil, ideaSynth, store)

	sig := testIdeaSignal()
	sig.Metadata["curated"] = false // wrong key on purpose; use the real one below
	delete(sig.Metadata, "aha_voter_org_count")
	sig.Type = "alert"

	if err := bridge.SynthesizeIdea(context.Background(), sig); err == nil {
		t.Fatal("expected error for a non-qualifying signal")
	}
	if len(store.specs) != 0 {
		t.Error("no spec should be saved for a non-qualifying signal")
	}
}
