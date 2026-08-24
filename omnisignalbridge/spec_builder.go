package omnisignalbridge

import (
	"fmt"

	"github.com/grokify/prism-roadmap/canvas"
)

// SpecInput bundles what BuildOpportunitySpec needs beyond the
// SolutionProposal itself: the problem-side content (Box 1), which differs
// by track -- a RootCause's title/symptoms/impact for the fix track, or a
// generic "customer-requested enhancement" framing for the idea track
// (Ideas are already solution-shaped and don't separately state a problem).
type SpecInput struct {
	ProblemStatement string
	Evidence         string
}

// specID returns the stable, generated typed ref used as
// canvas.OpportunitySpec.Metadata.ID / assessment.OpportunityRef.SpecID,
// matching this session's established typed-ref convention
// (omnisignal:<kind>:<id>).
func specID(proposal SolutionProposal) string {
	return fmt.Sprintf("omnisignal:%s:%s", proposal.SourceKind, proposal.SourceID)
}

// BuildOpportunitySpec populates Box 1 (Users & Problem) and Box 3 (Solution
// Ideas) of a canvas.OpportunitySpec from a SolutionProposal and its
// SpecInput. Boxes 2, 4-12 are left at zero value -- OpportunitySpec has no
// Validate() requiring them, and this bridge doesn't have content for them.
func BuildOpportunitySpec(proposal SolutionProposal, input SpecInput) *canvas.OpportunitySpec {
	id := specID(proposal)
	spec := canvas.NewOpportunitySpec(id, proposal.Title)

	spec.UsersAndProblem = canvas.OSUsersAndProblem{
		ProblemStatement: input.ProblemStatement,
		Evidence:         input.Evidence,
	}

	spec.SolutionIdeas = canvas.OSSolutionIdeas{
		Ideas: []canvas.OSSolutionIdea{
			{
				ID:          "idea-1",
				Name:        proposal.Title,
				Description: proposal.Description,
			},
		},
		RecommendedIdea: "idea-1",
		SelectionReason: proposal.Rationale,
	}

	return spec
}
