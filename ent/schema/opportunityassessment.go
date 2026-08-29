package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/grokify/prism-roadmap/assessment"
)

// OpportunityAssessment holds the canonical Opportunity Assessment IR
// (github.com/grokify/prism-roadmap/assessment.OpportunityAssessment) as
// the normative record, plus indexed projection columns rebuilt
// deterministically from it by the store layer at save time — never
// hand-written independently of it (prism-roadmap TRD: "omniroadmap
// relational fields = indexed/materialized projection"). Distinct from
// Item/ItemAugment: an Item is a provider-synced roadmap entry; an
// OpportunityAssessment is an evidence-backed judgment record produced by
// the rubric-driven prioritization system (INIT-PRISMROADMAP-001).
type OpportunityAssessment struct {
	ent.Schema
}

// Fields of the OpportunityAssessment.
func (OpportunityAssessment) Fields() []ent.Field {
	return []ent.Field{
		// id is the assessment ID (OA-NNN convention).
		field.String("id").MaxLen(64).NotEmpty(),

		// opportunity_spec_id / rmi_id mirror
		// assessment.OpportunityAssessment.Opportunity, indexed for
		// "all cycles for this opportunity" lookups.
		field.String("opportunity_spec_id").MaxLen(255).NotEmpty(),
		field.String("rmi_id").MaxLen(255).Optional(),
		field.String("title").MaxLen(512).Optional(),

		// Cycle fields flattened for filtering — "the current assessment
		// per opportunity" is the hot-path query for a compile run.
		field.Int("cycle_number").Default(1),
		field.Time("assessed_at"),
		field.String("supersedes_id").MaxLen(64).Optional(),
		field.Bool("current").Default(true),

		// Indexed projection columns. moscow_class/rice_score/
		// rice_computable/kano_category/mih_category are computed from
		// canonical at save time (RMI-OMNIROADMAP-001).
		// opportunity_rank_calculated/final are left unset until rank
		// materialization runs across the full corpus
		// (RMI-OMNIROADMAP-006) — ranking is inherently cross-opportunity
		// and cannot be derived from a single assessment in isolation.
		// compass_profile_id mirrors canonical.compass.profileId when a
		// COMPASS-RICE assessment is recorded — rice_score/rice_computable
		// are derived compass-first (assessment.ResolveCompassRICE) to
		// match ToRankInput's own precedence, matching prism-roadmap
		// v0.20.0's compass-first ranking (RMI-OMNIROADMAP-013).
		field.String("moscow_class").MaxLen(32).Optional(),
		field.Float("rice_score").Optional().Nillable(),
		field.Bool("rice_computable").Default(false),
		field.Int("opportunity_rank_calculated").Optional().Nillable(),
		field.Int("opportunity_rank_final").Optional().Nillable(),
		field.String("kano_category").MaxLen(64).Optional(),
		field.String("mih_category").MaxLen(64).Optional(),
		field.String("compass_profile_id").MaxLen(64).Optional(),

		// canonical is the full assessment.OpportunityAssessment, typed
		// directly so reads deserialize straight into the real Go struct
		// rather than a generic map — the normative record.
		field.JSON("canonical", assessment.OpportunityAssessment{}),

		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Indexes of the OpportunityAssessment.
func (OpportunityAssessment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("opportunity_spec_id"),
		index.Fields("opportunity_spec_id", "current"),
		index.Fields("rmi_id"),
		index.Fields("current"),
		index.Fields("moscow_class"),
		index.Fields("kano_category"),
		index.Fields("mih_category"),
		index.Fields("compass_profile_id"),
	}
}
