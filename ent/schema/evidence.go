package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/grokify/prism-roadmap/assessment"
)

// Evidence holds one independently referenceable
// assessment.Evidence record (github.com/grokify/prism-roadmap/assessment
// RMI-PRISMROADMAP-001) as the normative record, plus indexed projection
// columns for the staleness sweep (prism-roadmap PRD FR11). Reverse lookup
// ("which assessments cite this evidence") is intentionally NOT a
// persisted table here either — the same reasoning as
// PortfolioDimension's doc comment: OpportunityAssessment.Canonical
// already carries every citation, and
// assessment.OpportunityAssessment.EvidenceReferences() +
// assessment.NewEvidenceIndex already answer the query over the in-memory
// corpus (see store.DoltStore.EvidenceCitations).
type Evidence struct {
	ent.Schema
}

// Fields of the Evidence.
func (Evidence) Fields() []ent.Field {
	return []ent.Field{
		// id is the evidence identifier, conventionally EV-NNN.
		field.String("id").MaxLen(64).NotEmpty(),

		// Indexed projection columns, computed from canonical at save
		// time — the staleness sweep's hot-path filters.
		field.String("system").MaxLen(64).Optional(),
		field.String("sensitivity").MaxLen(32).Optional(),
		field.String("captured_by").MaxLen(255).Optional(),
		field.Time("captured_at").Optional().Nillable(),
		field.String("source_uri").MaxLen(2048).Optional(),
		field.Bool("verified").Default(false),

		// canonical is the full assessment.Evidence record, typed
		// directly — the normative record.
		field.JSON("canonical", assessment.Evidence{}),

		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the Evidence.
func (Evidence) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("system"),
		index.Fields("captured_at"),
		index.Fields("verified"),
	}
}
