// Package review implements the PM review gate: structured, auditable
// edits that flow back into the assessment IR (an override, or a new
// assessment cycle) — never a direct edit to a rendered/compiled report
// (prism-roadmap PRD FR13: "the rendered report is never directly edited;
// narrative-slot text is the sole PM-editable rendering element").
//
// Applying edits and recompiling are deliberately separate steps: Apply
// never touches a compile.Store's ReportDataset, so every dataset stays a
// pure function of the IR at the moment it was compiled. Callers apply
// edits, then call compile.Compile explicitly to see them reflected.
package review

import (
	"context"
	"fmt"

	"github.com/grokify/prism-roadmap/assessment"
)

// EditKind identifies what a review edit changes.
type EditKind string

const (
	// EditOverride records a governance override moving an opportunity to
	// an explicit final rank, with rationale — new evidence for the
	// ranking decision, never a reweighting of RICE/MoSCoW inputs to
	// manufacture a desired number (ideation doc).
	EditOverride EditKind = "override"

	// EditClearOverride removes a previously-recorded override, reverting
	// the opportunity to its calculated rank on the next compile.
	EditClearOverride EditKind = "clear_override"

	// EditAssessment records a new assessment cycle: a corrected effort
	// estimate, a reclassified dimension, or any other judgment change —
	// including a "defer" (a cycle whose MoSCoW resolves to Won't). This
	// is the mechanism for prism-roadmap PRD FR13's "corrected effort",
	// "reclassification", and "defer/suppress": all are new
	// evidence-backed judgments, not overrides, and belong in a new
	// NextCycle rather than mutating a past assessment (prism-roadmap TRD
	// D1) — there is no separate EditKind for defer because it is not a
	// structurally different persistence path, just a particular outcome
	// of a new cycle's MoSCoWAnswers.
	EditAssessment EditKind = "assessment"
)

// Edit is one structured, auditable review action. Exactly one of
// Override or Assessment is set, matching Kind.
type Edit struct {
	Kind EditKind

	// AssessmentID is required for EditClearOverride; ignored otherwise
	// (Override/Assessment already carry their own ID).
	AssessmentID string

	Override   *assessment.RankOverride
	Assessment *assessment.OpportunityAssessment
}

// Validate returns an error if the edit is not well-formed for its Kind.
func (e Edit) Validate() error {
	switch e.Kind {
	case EditOverride:
		if e.Override == nil {
			return fmt.Errorf("override edit requires Override")
		}
		return e.Override.Validate()
	case EditClearOverride:
		if e.AssessmentID == "" {
			return fmt.Errorf("clear_override edit requires AssessmentID")
		}
		return nil
	case EditAssessment:
		if e.Assessment == nil {
			return fmt.Errorf("assessment edit requires Assessment")
		}
		return e.Assessment.Validate()
	default:
		return fmt.Errorf("unknown edit kind %q", e.Kind)
	}
}

// Store is the persistence surface Apply needs.
type Store interface {
	SaveRankOverride(ctx context.Context, o assessment.RankOverride) error
	DeleteRankOverride(ctx context.Context, assessmentID string) error
	SaveOpportunityAssessment(ctx context.Context, a assessment.OpportunityAssessment) error
}

// Apply validates and persists one review edit.
func Apply(ctx context.Context, s Store, edit Edit) error {
	if err := edit.Validate(); err != nil {
		return fmt.Errorf("review: invalid edit: %w", err)
	}

	switch edit.Kind {
	case EditOverride:
		if err := s.SaveRankOverride(ctx, *edit.Override); err != nil {
			return fmt.Errorf("review: applying override: %w", err)
		}
	case EditClearOverride:
		if err := s.DeleteRankOverride(ctx, edit.AssessmentID); err != nil {
			return fmt.Errorf("review: clearing override: %w", err)
		}
	case EditAssessment:
		if err := s.SaveOpportunityAssessment(ctx, *edit.Assessment); err != nil {
			return fmt.Errorf("review: applying assessment edit: %w", err)
		}
	}
	return nil
}

// ApplyBatch applies a sequence of edits in order, stopping at the first
// failure. Callers typically follow a batch with a single compile.Compile
// call rather than recompiling after each individual edit.
func ApplyBatch(ctx context.Context, s Store, edits []Edit) error {
	for i, edit := range edits {
		if err := Apply(ctx, s, edit); err != nil {
			return fmt.Errorf("review: edit %d: %w", i, err)
		}
	}
	return nil
}
