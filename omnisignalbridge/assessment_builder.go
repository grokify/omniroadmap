package omnisignalbridge

import (
	"fmt"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
	"github.com/grokify/prism-roadmap/canvas"
)

// ReachInput bundles the evidence-backed reach estimate for an opportunity:
// a count of distinct known-affected customers (from a RootCause's
// Impact.AffectedCustomers, or an Idea's distinct voter-org count) and the
// signal IDs backing that count.
type ReachInput struct {
	// KnownAffectedCustomers is the count of distinct customers/accounts
	// directly known to be affected (support tickets) or requesting
	// (idea voters) -- a lower bound, not a market-wide percentage.
	KnownAffectedCustomers int

	// EvidenceIDs cite the signals this count is derived from. Required
	// whenever KnownAffectedCustomers > 0 (assessment.Reach.Validate()
	// rejects a nonzero fraction with no evidence).
	EvidenceIDs []string
}

// buildReach constructs an assessment.Reach from a ReachInput. Fraction is
// always 1.0 when KnownAffectedCustomers > 0: this is a known/measured
// count, not sampled against a larger population, so "reach" here means
// "100% of the known-affected population," with Population describing what
// was actually counted. A real total-addressable-market-relative fraction
// would need TAM data this bridge doesn't have -- see package doc comment.
func buildReach(input ReachInput) assessment.Reach {
	if input.KnownAffectedCustomers <= 0 {
		return assessment.Reach{}
	}
	return assessment.Reach{
		Fraction:    1.0,
		Population:  fmt.Sprintf("%d known affected/requesting customer accounts (lower bound, not market-wide)", input.KnownAffectedCustomers),
		Rationale:   "Derived directly from linked signal evidence; not extrapolated to a total addressable market.",
		EvidenceIDs: input.EvidenceIDs,
	}
}

// BuildOpportunityAssessment builds the first assessment cycle for spec.
// RICE is populated with a real, evidence-backed Reach but an unset
// EstimabilityGate, so assessment.ComputeRICE correctly reports
// Computable:false rather than fabricating a score from a guessed effort.
// MoSCoWAnswers/ImpactAnswers/ConfidenceAnswers are left empty --
// completing those requires the rubric/judge subsystem this bridge doesn't
// touch (RMI-OMNISIGNAL-030); they're filled in later via
// omniroadmap/review.EditAssessment.
func BuildOpportunityAssessment(spec canvas.OpportunitySpec, reach ReachInput, assessedAt time.Time) *assessment.OpportunityAssessment {
	id := "OA-" + spec.Metadata.ID
	ref := assessment.OpportunityRef{SpecID: spec.Metadata.ID}

	a := assessment.NewOpportunityAssessment(id, ref, spec.Metadata.Title, assessedAt)
	a.RICE = &assessment.RICEAssessment{
		Reach: buildReach(reach),
		// ImpactAnswers, ConfidenceAnswers, Effort: intentionally left
		// zero-value -- see doc comment.
	}
	return a
}
