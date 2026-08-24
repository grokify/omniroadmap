package omnisignalbridge

import (
	"testing"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
)

func testProposal() SolutionProposal {
	return SolutionProposal{
		Title:             "Serialize token refresh",
		Description:       "Add a mutex around the token refresh path.",
		Rationale:         "Directly addresses the race condition.",
		SourceKind:        SourceKindRootCause,
		SourceID:          "rc-1",
		EvidenceSignalIDs: []string{"sig-1", "sig-2"},
	}
}

func TestSpecID(t *testing.T) {
	got := specID(testProposal())
	want := "omnisignal:root_cause:rc-1"
	if got != want {
		t.Errorf("specID() = %q, want %q", got, want)
	}
}

func TestBuildOpportunitySpec(t *testing.T) {
	proposal := testProposal()
	spec := BuildOpportunitySpec(proposal, SpecInput{
		ProblemStatement: "SSO login fails intermittently",
		Evidence:         "3 tickets, 2 customers",
	})

	if spec.Metadata.ID != "omnisignal:root_cause:rc-1" {
		t.Errorf("Metadata.ID = %q, want omnisignal:root_cause:rc-1", spec.Metadata.ID)
	}
	if spec.Metadata.Title != proposal.Title {
		t.Errorf("Metadata.Title = %q, want %q", spec.Metadata.Title, proposal.Title)
	}
	if spec.UsersAndProblem.ProblemStatement != "SSO login fails intermittently" {
		t.Errorf("ProblemStatement = %q", spec.UsersAndProblem.ProblemStatement)
	}
	if spec.UsersAndProblem.Evidence != "3 tickets, 2 customers" {
		t.Errorf("Evidence = %q", spec.UsersAndProblem.Evidence)
	}
	if len(spec.SolutionIdeas.Ideas) != 1 {
		t.Fatalf("len(SolutionIdeas.Ideas) = %d, want 1", len(spec.SolutionIdeas.Ideas))
	}
	if spec.SolutionIdeas.Ideas[0].Name != proposal.Title {
		t.Errorf("Ideas[0].Name = %q, want %q", spec.SolutionIdeas.Ideas[0].Name, proposal.Title)
	}
	if spec.SolutionIdeas.RecommendedIdea != "idea-1" {
		t.Errorf("RecommendedIdea = %q, want idea-1", spec.SolutionIdeas.RecommendedIdea)
	}
	if spec.SolutionIdeas.SelectionReason != proposal.Rationale {
		t.Errorf("SelectionReason = %q, want %q", spec.SolutionIdeas.SelectionReason, proposal.Rationale)
	}
}

func TestBuildReach(t *testing.T) {
	t.Run("zero customers yields zero-value Reach", func(t *testing.T) {
		r := buildReach(ReachInput{KnownAffectedCustomers: 0})
		if r.Fraction != 0 || len(r.EvidenceIDs) != 0 {
			t.Errorf("buildReach(0) = %+v, want zero value", r)
		}
	})

	t.Run("nonzero customers yields Fraction=1.0 with evidence", func(t *testing.T) {
		r := buildReach(ReachInput{KnownAffectedCustomers: 3, EvidenceIDs: []string{"sig-1", "sig-2"}})
		if r.Fraction != 1.0 {
			t.Errorf("Fraction = %v, want 1.0", r.Fraction)
		}
		if len(r.EvidenceIDs) != 2 {
			t.Errorf("EvidenceIDs = %v, want 2 entries", r.EvidenceIDs)
		}
		if err := r.Validate(); err != nil {
			t.Errorf("Reach.Validate() = %v, want nil (evidence present for nonzero fraction)", err)
		}
	})
}

func TestBuildOpportunityAssessment(t *testing.T) {
	proposal := testProposal()
	spec := BuildOpportunitySpec(proposal, SpecInput{ProblemStatement: "problem"})
	assessedAt := time.Date(2026, 8, 22, 0, 0, 0, 0, time.UTC)

	a := BuildOpportunityAssessment(*spec, ReachInput{
		KnownAffectedCustomers: 2,
		EvidenceIDs:            []string{"sig-1"},
	}, assessedAt)

	if a.ID != "OA-"+spec.Metadata.ID {
		t.Errorf("ID = %q, want OA-%s", a.ID, spec.Metadata.ID)
	}
	if a.Opportunity.SpecID != spec.Metadata.ID {
		t.Errorf("Opportunity.SpecID = %q, want %q", a.Opportunity.SpecID, spec.Metadata.ID)
	}
	if a.Opportunity.RMIID != "" {
		t.Errorf("Opportunity.RMIID = %q, want empty (no Item exists yet)", a.Opportunity.RMIID)
	}
	if a.Cycle.Number != 1 || !a.Cycle.Current {
		t.Errorf("Cycle = %+v, want Number=1 Current=true", a.Cycle)
	}
	if !a.Cycle.AssessedAt.Equal(assessedAt) {
		t.Errorf("Cycle.AssessedAt = %v, want %v", a.Cycle.AssessedAt, assessedAt)
	}

	// The core contract: RICE must be honestly not-computable, never a
	// fabricated score, because ImpactAnswers/ConfidenceAnswers/Effort.Gate
	// are left empty by construction.
	if len(a.RICE.ImpactAnswers) != 0 || len(a.RICE.ConfidenceAnswers) != 0 {
		t.Errorf("RICE answers should be empty by construction: %+v", a.RICE)
	}
	if a.RICE.Effort.Gate.Passed() {
		t.Error("Effort.Gate should not be passed by construction")
	}
	result := assessment.ComputeRICE(*a.RICE)
	if result.Computable {
		t.Errorf("ComputeRICE = %+v, want Computable=false", result)
	}
	if len(a.MoSCoWAnswers) != 0 {
		t.Errorf("MoSCoWAnswers should be empty by construction: %v", a.MoSCoWAnswers)
	}

	if err := a.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil (assessment should be structurally valid even though RICE is not computable)", err)
	}
}
