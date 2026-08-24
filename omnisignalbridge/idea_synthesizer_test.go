package omnisignalbridge

import (
	"testing"

	"github.com/plexusone/omnisignal"
	"github.com/plexusone/signal-spec/pkg/signal"
)

func testIdeaSignal() signal.Signal {
	return signal.Signal{
		ID:          "aha-PROJ-I-1",
		Type:        signal.TypeEnhancementRequest,
		Summary:     "Add dark mode",
		Description: "Users have requested a dark theme for the dashboard.",
		Metadata: map[string]any{
			omnisignal.MetaCurated: true,
			signal.MetaVotes:       12,
			"aha_voter_org_count":  3,
		},
	}
}

func TestIdeaSynthesizer_Qualifies(t *testing.T) {
	synth := NewIdeaSynthesizer()

	if !synth.Qualifies(testIdeaSignal()) {
		t.Error("curated enhancement_request signal should qualify")
	}

	notCurated := testIdeaSignal()
	notCurated.Metadata[omnisignal.MetaCurated] = false
	if synth.Qualifies(notCurated) {
		t.Error("non-curated signal should not qualify")
	}

	wrongType := testIdeaSignal()
	wrongType.Type = signal.TypeAlert
	if synth.Qualifies(wrongType) {
		t.Error("non-enhancement-request signal should not qualify")
	}
}

func TestIdeaSynthesizer_Synthesize(t *testing.T) {
	synth := NewIdeaSynthesizer()
	sig := testIdeaSignal()

	proposal := synth.Synthesize(sig)

	if proposal.Title != sig.Summary {
		t.Errorf("Title = %q, want %q", proposal.Title, sig.Summary)
	}
	if proposal.Description != sig.Description {
		t.Errorf("Description = %q, want %q", proposal.Description, sig.Description)
	}
	if proposal.SourceKind != SourceKindIdea {
		t.Errorf("SourceKind = %q, want %q", proposal.SourceKind, SourceKindIdea)
	}
	if proposal.SourceID != sig.ID {
		t.Errorf("SourceID = %q, want %q", proposal.SourceID, sig.ID)
	}
	if len(proposal.EvidenceSignalIDs) != 1 || proposal.EvidenceSignalIDs[0] != sig.ID {
		t.Errorf("EvidenceSignalIDs = %v, want [%s]", proposal.EvidenceSignalIDs, sig.ID)
	}
}
