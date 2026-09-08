// moscowCmd views and sets an opportunity's MoSCoW tier -- always resolved
// from evidence-backed ladder answers (assessment.ResolveMoSCoWPriority),
// never assigned as a bare tier.
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/grokify/prism-roadmap/assessment"
	"github.com/spf13/cobra"

	"github.com/grokify/omniroadmap/review"
)

func moscowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "moscow",
		Short: "View or set an opportunity's MoSCoW tier via evidence-backed ladder answers",
	}
	cmd.AddCommand(moscowGetCmd(), moscowSetCmd())
	return cmd
}

func moscowGetCmd() *cobra.Command {
	flags := &storeFlags{}
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "get <spec-id>",
		Short: "Show the resolved MoSCoW tier and the ladder answers behind it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			a, err := currentAssessmentForSpec(cmd.Context(), s, args[0])
			if err != nil {
				return err
			}
			if a == nil {
				return fmt.Errorf("no current assessment recorded for spec %q", args[0])
			}

			if jsonOutput {
				return printJSON(cmd, struct {
					SpecID  string                       `json:"specId"`
					MoSCoW  string                       `json:"moscow"`
					Answers []assessment.ThresholdAnswer `json:"answers"`
				}{SpecID: a.Opportunity.SpecID, MoSCoW: a.MoSCoW().String(), Answers: a.MoSCoWAnswers})
			}

			fmt.Fprintf(cmd.OutOrStdout(), "MoSCoW: %s\n\n", a.MoSCoW())
			if len(a.MoSCoWAnswers) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No ladder answers recorded yet.")
				return nil
			}
			w := newTabWriter(cmd)
			fmt.Fprintln(w, "LEVEL\tSATISFIED\tCRITERION MET\tEVIDENCE")
			for _, ans := range a.MoSCoWAnswers {
				fmt.Fprintf(w, "%s\t%v\t%s\t%s\n", ans.LevelID, ans.Satisfied, dashIfEmpty(ans.CriterionMet), strings.Join(ans.EvidenceIDs, ","))
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON instead of a table")
	flags.register(cmd)
	return cmd
}

func moscowSetCmd() *cobra.Command {
	flags := &storeFlags{}
	var level, rationale, assessedAtStr string
	var criterionIndex int
	var evidenceIDs []string

	cmd := &cobra.Command{
		Use:   "set <spec-id>",
		Short: "Record an evidence-backed MoSCoW ladder answer as a new assessment cycle",
		Long: `Records a satisfied answer for one MoSCoW ladder level (must, should, or
could), citing which of the level's criteria was met and what evidence
supports it -- MoSCoW is never set as a bare tier, only resolved from
evidence-backed ladder answers (assessment.ResolveMoSCoWPriority).
Persisted through review.Apply as a new assessment cycle, carrying every
other field (Compass, RICE, dimensions, ...) forward unchanged -- never a
mutation of a past cycle, and never a silent drop of unrelated judgment
data.

Run "moscow get <spec-id>" first to see the ladder's criteria list and
choose --criterion by its 1-indexed position.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specID := args[0]
			if level == "" {
				return fmt.Errorf("--level is required (must, should, or could)")
			}
			if rationale == "" {
				return fmt.Errorf("--rationale is required")
			}
			if len(evidenceIDs) == 0 {
				return fmt.Errorf("--evidence is required -- a MoSCoW answer cannot be satisfied without cited evidence")
			}

			ladderLevel := assessment.MoSCoWLadder().LevelByID(strings.ToLower(level))
			if ladderLevel == nil {
				return fmt.Errorf("unknown MoSCoW level %q (want must, should, or could)", level)
			}
			criterionMet := ""
			if criterionIndex > 0 {
				if criterionIndex > len(ladderLevel.Criteria) {
					return fmt.Errorf("--criterion %d out of range (level %q has %d criteria; run `moscow get` to see them)", criterionIndex, level, len(ladderLevel.Criteria))
				}
				criterionMet = ladderLevel.Criteria[criterionIndex-1]
			}

			assessedAt, err := parseAssessedAt(assessedAtStr)
			if err != nil {
				return err
			}

			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			ctx := cmd.Context()
			prior, err := currentAssessmentForSpec(ctx, s, specID)
			if err != nil {
				return err
			}

			answer := assessment.ThresholdAnswer{
				LevelID: ladderLevel.ID, Satisfied: true, CriterionMet: criterionMet,
				Rationale: rationale, EvidenceIDs: evidenceIDs,
			}

			var next *assessment.OpportunityAssessment
			if prior == nil {
				next = assessment.NewOpportunityAssessment("OA-"+specID, assessment.OpportunityRef{SpecID: specID}, specID, assessedAt)
				next.MoSCoWAnswers = []assessment.ThresholdAnswer{answer}
			} else {
				next = carryForwardNextCycle(prior, fmt.Sprintf("OA-%s-c%d", specID, prior.Cycle.Number+1), assessedAt)
				next.MoSCoWAnswers = upsertMoSCoWAnswer(next.MoSCoWAnswers, answer)
			}

			if err := review.Apply(ctx, s, review.Edit{Kind: review.EditAssessment, Assessment: next}); err != nil {
				return err
			}
			if err := s.Commit(ctx, fmt.Sprintf("omniroadmap moscow set: %s -> %s", specID, ladderLevel.ID)); err != nil {
				return err
			}
			return printJSON(cmd, next)
		},
	}
	cmd.Flags().StringVar(&level, "level", "", "MoSCoW ladder level: must, should, or could (required)")
	cmd.Flags().IntVar(&criterionIndex, "criterion", 0, "1-indexed criterion from the level's list (see the moscow get command)")
	cmd.Flags().StringVar(&rationale, "rationale", "", "Why this criterion is met (required)")
	cmd.Flags().StringSliceVar(&evidenceIDs, "evidence", nil, "Evidence IDs supporting this answer (required, comma-separated or repeatable)")
	cmd.Flags().StringVar(&assessedAtStr, "assessed-at", "", "Assessment timestamp (RFC3339, default now)")
	flags.register(cmd)
	return cmd
}

// upsertMoSCoWAnswer replaces the answer for next.LevelID if one already
// exists in answers, or appends it otherwise -- re-running `moscow set` for
// the same level updates that level's answer rather than duplicating it.
func upsertMoSCoWAnswer(answers []assessment.ThresholdAnswer, next assessment.ThresholdAnswer) []assessment.ThresholdAnswer {
	for i, a := range answers {
		if a.LevelID == next.LevelID {
			out := append([]assessment.ThresholdAnswer{}, answers...)
			out[i] = next
			return out
		}
	}
	return append(append([]assessment.ThresholdAnswer{}, answers...), next)
}

// carryForwardNextCycle creates the next assessment cycle for prior's
// opportunity, carrying forward every judgment field unchanged -- the
// caller then overrides just the one field their command updates.
// OpportunityAssessment.NextCycle itself only carries Opportunity/Title/
// Cycle forward by design (see compassbridge.NextCycleWithCompass's own
// doc comment for the same reasoning): a cycle update to one field must
// never silently drop another field's prior state.
func carryForwardNextCycle(prior *assessment.OpportunityAssessment, id string, assessedAt time.Time) *assessment.OpportunityAssessment {
	next := prior.NextCycle(id, assessedAt)
	next.Judge = prior.Judge
	next.MoSCoWAnswers = append([]assessment.ThresholdAnswer{}, prior.MoSCoWAnswers...)
	next.RICE = prior.RICE
	next.Compass = prior.Compass
	next.Dimensions = append([]assessment.DimensionAssignment{}, prior.Dimensions...)
	next.Contributions = append([]assessment.OKRContribution{}, prior.Contributions...)
	next.Capabilities = append([]assessment.CapabilityReference{}, prior.Capabilities...)
	return next
}
