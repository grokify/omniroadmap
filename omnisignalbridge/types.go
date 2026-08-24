// Package omnisignalbridge turns omnisignal RootCauses (clustered support
// tickets) and curated enhancement-request Signals (e.g. Aha Ideas) into
// canvas.OpportunitySpec + assessment.OpportunityAssessment records
// persisted in omniroadmap, so they show up as candidate opportunities in
// the normal compile -> review -> materialize ranking pipeline.
//
// This package -- not omnisignal itself -- owns the dependency on
// prism-roadmap's ranking types (canvas.OpportunitySpec,
// assessment.OpportunityAssessment), mirroring the reasoning already used
// for aha-studio/omnisignalcache: keep omnisignal decoupled from any one
// downstream consumer's types.
//
// No omniroadmap.Item is ever created here -- assessment.OpportunityRef.RMIID
// is optional by design ("if one has been created"), and nothing validates
// Opportunity.SpecID/RMIID against a persisted Item. Filing a real Item
// (e.g. an actual Jira epic or Aha feature) is a separate, human-driven
// "promotion" action, out of scope for this package.
//
// RICE scoring is deliberately left not-computable
// (assessment.RICEScoreResult.Computable == false): ImpactAnswers/
// ConfidenceAnswers require the rubric/judge subsystem
// (structured-evaluation/claims), which this package doesn't touch. Reach is
// populated from real signal/voter evidence; Effort's EstimabilityGate is
// left unset. Completing the judgment is deferred to
// omniroadmap/review.EditAssessment, which already exists for exactly this
// "come back and finish this" purpose.
package omnisignalbridge

// SolutionProposal is the shared intermediate produced by both synthesis
// tracks (RootCauseSynthesizer for support-ticket clusters, IdeaSynthesizer
// for curated enhancement-request signals) before being turned into a
// canvas.OpportunitySpec.
type SolutionProposal struct {
	// Title is a short, actionable name for the proposed solution.
	Title string

	// Description explains what the solution does.
	Description string

	// Rationale explains why this solution addresses the underlying
	// problem -- maps to OSSolutionIdeas.SelectionReason.
	Rationale string

	// SourceKind is "root_cause" or "idea" -- which track produced this.
	SourceKind string

	// SourceID is the rootcause.RootCause.ID or signal.Signal.ID this
	// proposal was derived from.
	SourceID string

	// EvidenceSignalIDs are the signal.Signal IDs backing this proposal
	// (a RootCause's linked signals, or the idea signal's own ID).
	EvidenceSignalIDs []string
}

const (
	SourceKindRootCause = "root_cause"
	SourceKindIdea      = "idea"
)
