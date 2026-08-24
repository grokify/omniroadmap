package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RankOverride holds a governance decision to move an opportunity away
// from its calculated rank (github.com/grokify/prism-roadmap/assessment
// RankOverride, RMI-PRISMROADMAP-004) — recorded as evidence, never
// applied by quietly reweighting RICE/MoSCoW inputs to manufacture a
// desired number. One row per assessment ID: an assessment either has an
// active override or it doesn't, matching
// assessment.ApplyOverrides' keyed-by-AssessmentID lookup.
type RankOverride struct {
	ent.Schema
}

// Fields of the RankOverride.
func (RankOverride) Fields() []ent.Field {
	return []ent.Field{
		// id is the OpportunityAssessment.ID this override applies to.
		field.String("id").MaxLen(64).NotEmpty(),
		field.Int("final_rank"),
		field.Text("rationale").NotEmpty(),
		field.String("approved_by").MaxLen(255).NotEmpty(),
		field.JSON("evidence_ids", []string{}).Optional(),

		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the RankOverride.
func (RankOverride) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
	}
}
