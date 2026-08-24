package omnisignalbridge

import (
	"github.com/plexusone/omnisignal"
	"github.com/plexusone/signal-spec/pkg/signal"
)

// IdeaSynthesizer builds a SolutionProposal directly from a curated
// enhancement-request signal (e.g. an Aha Idea) -- deterministic, no LLM
// call. An Idea's own Summary/Description already *are* the proposed
// solution text (an idea is already solution-shaped, unlike a RootCause,
// which is problem-shaped), so this is a field mapping, not a synthesis
// step.
type IdeaSynthesizer struct{}

// NewIdeaSynthesizer creates an IdeaSynthesizer.
func NewIdeaSynthesizer() *IdeaSynthesizer {
	return &IdeaSynthesizer{}
}

// Qualifies reports whether sig is eligible for the idea track: curated and
// an enhancement request. Signals that don't qualify should go through
// RootCauseSynthesizer via the consolidate pipeline instead (or not be
// synthesized at all).
func (s *IdeaSynthesizer) Qualifies(sig signal.Signal) bool {
	if sig.Type != signal.TypeEnhancementRequest {
		return false
	}
	curated, _ := sig.Metadata[omnisignal.MetaCurated].(bool)
	return curated
}

// Synthesize builds a SolutionProposal from a curated idea signal. Callers
// should check Qualifies first.
func (s *IdeaSynthesizer) Synthesize(sig signal.Signal) SolutionProposal {
	return SolutionProposal{
		Title:             sig.Summary,
		Description:       sig.Description,
		SourceKind:        SourceKindIdea,
		SourceID:          sig.ID,
		EvidenceSignalIDs: []string{sig.ID},
	}
}
