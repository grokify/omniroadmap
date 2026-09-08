// Package compassbridge turns a compass-rice judge.Output into a
// prism-roadmap assessment.CompassAssessment, and implements the two-phase
// (LLM-proposed, PM-confirmed) profile assignment workflow — the runtime
// half of INIT-OMNIROADMAP-001's COMPASS-RICE integration, mirroring
// omnisignalbridge's role of owning the dependency on prism-roadmap's
// ranking types on behalf of an upstream producer.
//
// Ingest never trusts a judge's self-reported confidence: it compares the
// Confidence tier compass-rice's Normalizer derived from the evidence
// struct's self-reported source counts against the Confidence tier
// compass-rice's provenance package derives independently from the
// judge's actual verified claims, and rejects the assessment if the
// evidence struct claims more confidence than the claims support.
package compassbridge

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ProductBuildersHQ/compass-rice/catalog"
	"github.com/ProductBuildersHQ/compass-rice/judge"
	"github.com/ProductBuildersHQ/compass-rice/provenance"
	"github.com/ProductBuildersHQ/compass-rice/rice"
	"github.com/grokify/prism-roadmap/assessment"
)

// IngestError wraps an Ingest failure that a repair pass could address,
// carrying compass-rice's own RepairPrompts() for the reason codes attached
// to the judge output's categories.
type IngestError struct {
	err           error
	RepairPrompts []string
}

func (e *IngestError) Error() string { return e.err.Error() }
func (e *IngestError) Unwrap() error { return e.err }

// Ingest converts a compass-rice judge.Output into a CompassAssessment:
// normalizes the evidence via the compass-rice catalog, verifies the
// evidence's self-reported confidence doesn't exceed what the judge's
// verified claims actually support, and captures whether the assessment
// needs human review before it can be trusted for scoring.
func Ingest(output judge.Output) (assessment.CompassAssessment, error) {
	evidenceJSON, err := json.Marshal(output.Evidence)
	if err != nil {
		return assessment.CompassAssessment{}, fmt.Errorf("compassbridge: marshal evidence: %w", err)
	}

	normalized, err := catalog.Normalize(output.ProfileID, output.Evidence)
	if err != nil {
		return assessment.CompassAssessment{}, fmt.Errorf("compassbridge: normalize: %w", err)
	}

	claimsConfidence := provenance.Confidence(output.Claims)
	if normalized.Confidence > claimsConfidence {
		return assessment.CompassAssessment{}, &IngestError{
			err: fmt.Errorf(
				"compassbridge: evidence-derived confidence %s exceeds claims-derived confidence %s -- "+
					"evidence source counts must be consistent with verified claims",
				normalized.Confidence.Label(), claimsConfidence.Label(),
			),
			RepairPrompts: output.RepairPrompts(),
		}
	}

	c := assessment.CompassAssessment{
		ProfileID:        output.ProfileID,
		EvidenceJSON:     evidenceJSON,
		Normalized:       normalized,
		Categories:       output.Categories,
		Claims:           output.Claims,
		NeedsHumanReview: output.NeedsHumanReview(),
	}
	if err := c.Validate(); err != nil {
		return assessment.CompassAssessment{}, fmt.Errorf("compassbridge: %w", err)
	}
	return c, nil
}

// IngestHumanEvidence converts human-entered raw evidence directly into a
// CompassAssessment, bypassing the judge.Output/confidence-integrity path:
// a human who typed the evidence themselves is trusted at face value (no
// claims to cross-check), and the resulting assessment is marked as
// already human-reviewed -- there is no pending judge output to accept.
func IngestHumanEvidence(profileID rice.ProfileID, evidenceJSON json.RawMessage, enteredBy string, enteredAt time.Time) (assessment.CompassAssessment, error) {
	normalized, err := catalog.NormalizeJSON(profileID, evidenceJSON)
	if err != nil {
		return assessment.CompassAssessment{}, fmt.Errorf("compassbridge: normalize: %w", err)
	}

	c := assessment.CompassAssessment{
		ProfileID:    profileID,
		EvidenceJSON: evidenceJSON,
		Normalized:   normalized,
		HumanReview: &assessment.CompassHumanReview{
			ReviewedBy: enteredBy,
			ReviewedAt: enteredAt,
			Note:       "entered directly by a human; no judge output to review",
		},
	}
	if err := c.Validate(); err != nil {
		return assessment.CompassAssessment{}, fmt.Errorf("compassbridge: %w", err)
	}
	return c, nil
}

// FirstCycleWithCompass creates the first assessment cycle for an
// opportunity, with a COMPASS-RICE assessment already attached.
func FirstCycleWithCompass(id string, ref assessment.OpportunityRef, title string, assessedAt time.Time, c assessment.CompassAssessment) *assessment.OpportunityAssessment {
	a := assessment.NewOpportunityAssessment(id, ref, title, assessedAt)
	a.Compass = &c
	return a
}

// NextCycleWithCompass creates the next assessment cycle for prev's
// opportunity, with a COMPASS-RICE assessment already attached — the
// mechanism for re-scoring an opportunity (a corrected evidence document,
// or a PM's HumanReview acceptance) without mutating a past cycle.
//
// prev.NextCycle itself only carries forward Opportunity/Title/Cycle (every
// other field starts blank, so a caller must explicitly re-supply what a
// genuinely new judgment changed). Since NextCycleWithCompass only intends
// to change Compass, it carries every other field forward unchanged first
// — otherwise re-scoring an opportunity would silently drop its MoSCoW
// answers, dimensions, OKR contributions, and capability references.
func NextCycleWithCompass(prev *assessment.OpportunityAssessment, id string, assessedAt time.Time, c assessment.CompassAssessment) *assessment.OpportunityAssessment {
	next := prev.NextCycle(id, assessedAt)
	next.Judge = prev.Judge
	next.MoSCoWAnswers = append([]assessment.ThresholdAnswer{}, prev.MoSCoWAnswers...)
	next.RICE = prev.RICE
	next.Dimensions = append([]assessment.DimensionAssignment{}, prev.Dimensions...)
	next.Contributions = append([]assessment.OKRContribution{}, prev.Contributions...)
	next.Capabilities = append([]assessment.CapabilityReference{}, prev.Capabilities...)
	next.Compass = &c
	return next
}

// ProposeProfile creates a proposed profile assignment (the judge phase of
// the two-phase workflow) and validates it before returning.
func ProposeProfile(specID string, profileID rice.ProfileID, rationale, proposedBy string) (assessment.ProfileAssignment, error) {
	p := assessment.ProposeProfileAssignment(specID, profileID, rationale, proposedBy)
	if err := p.Validate(); err != nil {
		return assessment.ProfileAssignment{}, fmt.Errorf("compassbridge: %w", err)
	}
	return p, nil
}

// ConfirmProfile confirms a proposed profile assignment (the PM phase).
// Returns an error if current is not currently proposed — an already
// confirmed or rejected assignment must go through a new ProposeProfile
// cycle before it can be confirmed again.
func ConfirmProfile(current assessment.ProfileAssignment, confirmedBy string, confirmedAt time.Time) (assessment.ProfileAssignment, error) {
	if current.Status != assessment.ProfileAssignmentProposed {
		return assessment.ProfileAssignment{}, fmt.Errorf("compassbridge: cannot confirm assignment in status %q, want %q", current.Status, assessment.ProfileAssignmentProposed)
	}
	confirmed := current.Confirm(confirmedBy, confirmedAt)
	if err := confirmed.Validate(); err != nil {
		return assessment.ProfileAssignment{}, fmt.Errorf("compassbridge: %w", err)
	}
	return confirmed, nil
}

// RejectProfile rejects a proposed profile assignment, recording why (e.g.
// the judge proposed the wrong profile and a new proposal is needed).
// Returns an error if current is not currently proposed.
func RejectProfile(current assessment.ProfileAssignment, rejectedBy string, rejectedAt time.Time, rationale string) (assessment.ProfileAssignment, error) {
	if current.Status != assessment.ProfileAssignmentProposed {
		return assessment.ProfileAssignment{}, fmt.Errorf("compassbridge: cannot reject assignment in status %q, want %q", current.Status, assessment.ProfileAssignmentProposed)
	}
	rejected := current.Reject(rejectedBy, rejectedAt, rationale)
	if err := rejected.Validate(); err != nil {
		return assessment.ProfileAssignment{}, fmt.Errorf("compassbridge: %w", err)
	}
	return rejected, nil
}

// AssignProfileDirectly creates and immediately confirms a profile
// assignment in one step — the path for a PM who selects a profile
// directly, without a prior LLM proposal.
func AssignProfileDirectly(specID string, profileID rice.ProfileID, rationale, confirmedBy string, confirmedAt time.Time) (assessment.ProfileAssignment, error) {
	p, err := ProposeProfile(specID, profileID, rationale, confirmedBy)
	if err != nil {
		return assessment.ProfileAssignment{}, err
	}
	return ConfirmProfile(p, confirmedBy, confirmedAt)
}
