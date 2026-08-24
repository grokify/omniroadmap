package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/grokify/prism-roadmap/canvas"
)

// OpportunitySpec holds the canonical canvas.OpportunitySpec IR (the
// normative record) plus indexed projection columns rebuilt deterministically
// from it at save time — same discipline as OpportunityAssessment/Evidence
// ("omniroadmap relational fields = indexed/materialized projection").
//
// Nothing in omniroadmap persisted OpportunitySpec before this schema:
// assessment.OpportunityAssessment only ever references a spec by
// Opportunity.SpecID (a string), by design ("the spec is a human-authored
// discovery document with its own lifecycle" — prism-roadmap
// assessment.OpportunityRef doc comment). This table is that missing home.
type OpportunitySpec struct {
	ent.Schema
}

// Fields of the OpportunitySpec.
func (OpportunitySpec) Fields() []ent.Field {
	return []ent.Field{
		// id mirrors canvas.OpportunitySpec.Metadata.ID — the same ID
		// assessment.OpportunityRef.SpecID references.
		field.String("id").MaxLen(255).NotEmpty(),

		field.String("title").MaxLen(512).Optional(),

		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),

		// canonical is the full canvas.OpportunitySpec, typed directly so
		// reads deserialize straight into the real Go struct — the
		// normative record, same pattern as OpportunityAssessment.canonical
		// and Evidence.canonical.
		field.JSON("canonical", canvas.OpportunitySpec{}),
	}
}

// Indexes of the OpportunitySpec.
func (OpportunitySpec) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("title"),
	}
}
