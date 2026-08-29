package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ProfileAssignment holds the two-phase (LLM-proposed, PM-confirmed)
// primary COMPASS-RICE investment thesis for one opportunity
// (github.com/grokify/prism-roadmap/assessment.ProfileAssignment,
// RMI-PRISMROADMAP-017). One row per opportunity spec, matching
// RankOverride's one-row-per-key shape: an opportunity has at most one
// current profile assignment (proposed, confirmed, or rejected) at a time.
// Unlike OpportunityAssessment/Evidence, ProfileAssignment has no nested
// sub-structures that would lose information when flattened to columns, so
// it is fully decomposed rather than carrying a canonical JSON blob —
// following RankOverride's precedent, not the hybrid pattern.
type ProfileAssignment struct {
	ent.Schema
}

// Fields of the ProfileAssignment.
func (ProfileAssignment) Fields() []ent.Field {
	return []ent.Field{
		// id is the canvas.OpportunitySpec.Metadata.ID this assignment
		// applies to.
		field.String("id").MaxLen(255).NotEmpty(),

		field.String("profile_id").MaxLen(64).NotEmpty(),
		field.JSON("secondary", []string{}).Optional(),
		field.Text("rationale").NotEmpty(),
		field.String("proposed_by").MaxLen(255).NotEmpty(),
		field.String("status").MaxLen(32).NotEmpty(),
		field.String("confirmed_by").MaxLen(255).Optional(),
		field.Time("confirmed_at").Optional().Nillable(),
		field.JSON("evidence_ids", []string{}).Optional(),

		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the ProfileAssignment.
func (ProfileAssignment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("profile_id"),
		index.Fields("status"),
	}
}
