package omnisignalbridge

import (
	"context"
	"fmt"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
	"github.com/grokify/prism-roadmap/canvas"
	"github.com/plexusone/signal-spec/pkg/rootcause"
	"github.com/plexusone/signal-spec/pkg/signal"
)

// Store is the subset of omniroadmap/store.DoltStore this package needs.
// Kept as a narrow interface (rather than depending on *store.DoltStore
// directly) so it can be faked in tests without a real Dolt server.
type Store interface {
	SaveOpportunitySpec(ctx context.Context, spec canvas.OpportunitySpec) error
	SaveOpportunityAssessment(ctx context.Context, a assessment.OpportunityAssessment) error
}

// Bridge orchestrates synthesis (fix-track or idea-track) into persisted
// canvas.OpportunitySpec + assessment.OpportunityAssessment records.
type Bridge struct {
	rootCauseSynth *RootCauseSynthesizer
	ideaSynth      *IdeaSynthesizer
	store          Store
}

// NewBridge creates a Bridge. rootCauseSynth may be nil if only the idea
// track is needed (and vice versa for ideaSynth), so callers that only care
// about one track don't need to construct an LLM client for the other.
func NewBridge(rootCauseSynth *RootCauseSynthesizer, ideaSynth *IdeaSynthesizer, s Store) *Bridge {
	return &Bridge{rootCauseSynth: rootCauseSynth, ideaSynth: ideaSynth, store: s}
}

// IsEligibleRootCause reports whether rc qualifies for fix-track synthesis.
//
// Deviation from the original plan: the plan called for gating on
// consolidate.Reviewer/ReviewStatus (a formal human-approval workflow), but
// that state is tracked only by whichever Reviewer a Pipeline run was
// configured with (e.g. MemoryReviewer) -- it isn't persisted by
// consolidate.Store (including store/sqlite), so there's nothing durable to
// query it from yet. Gating on rc.Status != StatusArchived instead (treat
// "not dismissed" as eligible) until store/sqlite grows persisted Reviewer
// state, at which point this should switch to the real approval check.
func IsEligibleRootCause(rc rootcause.RootCause) bool {
	return rc.Status != rootcause.StatusArchived
}

// SynthesizeRootCause runs the fix track for an eligible RootCause: LLM
// drafts a SolutionProposal from rc's own evidence, builds a spec +
// first-cycle assessment, and persists both.
func (b *Bridge) SynthesizeRootCause(ctx context.Context, rc rootcause.RootCause) error {
	if b.rootCauseSynth == nil {
		return fmt.Errorf("omnisignalbridge: no RootCauseSynthesizer configured")
	}
	if !IsEligibleRootCause(rc) {
		return fmt.Errorf("omnisignalbridge: root cause %s is not eligible (status=%s)", rc.ID, rc.Status)
	}

	proposal, err := b.rootCauseSynth.Synthesize(ctx, rc)
	if err != nil {
		return fmt.Errorf("synthesizing solution for root cause %s: %w", rc.ID, err)
	}

	input := SpecInput{
		ProblemStatement: rc.Title,
		Evidence:         rc.Description,
	}
	spec := BuildOpportunitySpec(proposal, input)

	assessment := BuildOpportunityAssessment(*spec, ReachInput{
		KnownAffectedCustomers: rc.Impact.AffectedCustomers,
		EvidenceIDs:            rc.SignalIDs,
	}, time.Now())

	return b.persist(ctx, *spec, *assessment)
}

// SynthesizeIdea runs the idea track for a curated enhancement-request
// signal: deterministically maps it to a SolutionProposal, builds a spec +
// first-cycle assessment, and persists both. Returns an error if sig
// doesn't qualify (see IdeaSynthesizer.Qualifies) -- callers filtering a
// batch of signals should check Qualifies themselves to skip non-qualifying
// signals rather than treating them as errors.
func (b *Bridge) SynthesizeIdea(ctx context.Context, sig signal.Signal) error {
	if b.ideaSynth == nil {
		return fmt.Errorf("omnisignalbridge: no IdeaSynthesizer configured")
	}
	if !b.ideaSynth.Qualifies(sig) {
		return fmt.Errorf("omnisignalbridge: signal %s does not qualify for the idea track", sig.ID)
	}

	proposal := b.ideaSynth.Synthesize(sig)

	input := SpecInput{
		ProblemStatement: fmt.Sprintf("Customer-requested enhancement: %s", sig.Summary),
		Evidence:         ideaEvidenceText(sig),
	}
	spec := BuildOpportunitySpec(proposal, input)

	assessment := BuildOpportunityAssessment(*spec, ReachInput{
		KnownAffectedCustomers: ideaVoterOrgCount(sig),
		EvidenceIDs:            []string{sig.ID},
	}, time.Now())

	return b.persist(ctx, *spec, *assessment)
}

func (b *Bridge) persist(ctx context.Context, spec canvas.OpportunitySpec, a assessment.OpportunityAssessment) error {
	if err := b.store.SaveOpportunitySpec(ctx, spec); err != nil {
		return fmt.Errorf("saving opportunity spec %s: %w", spec.Metadata.ID, err)
	}
	if err := b.store.SaveOpportunityAssessment(ctx, a); err != nil {
		return fmt.Errorf("saving opportunity assessment %s: %w", a.ID, err)
	}
	return nil
}

// ideaVoterOrgCount reads aha_voter_org_count from Signal.Metadata
// (populated by aha-studio/omnisignalcache), defaulting to 0 if absent.
func ideaVoterOrgCount(sig signal.Signal) int {
	if sig.Metadata == nil {
		return 0
	}
	switch v := sig.Metadata["aha_voter_org_count"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// ideaEvidenceText summarizes an idea's voter evidence for the spec's
// Evidence field (aggregate counts only, never raw voter PII, matching this
// session's established PII stance).
func ideaEvidenceText(sig signal.Signal) string {
	votes, _ := sig.Metadata[signal.MetaVotes].(int)
	orgs := ideaVoterOrgCount(sig)
	return fmt.Sprintf("%d votes across %d distinct known customer organizations.", votes, orgs)
}
