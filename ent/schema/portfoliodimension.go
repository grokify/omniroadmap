package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PortfolioDimension holds a versioned portfolio dimension definition —
// Kano, Market Investment Horizon (both registered as built-in at
// migrate/seed time), or an organization-defined custom dimension (e.g. a
// "2026 Strategic Priority: AI/Growth/Excellence" category). Registering a
// new custom dimension is an INSERT into this table and DimensionOption,
// never a schema migration (prism-roadmap PRD FR4).
//
// This table is deliberately just the definition registry — it does NOT
// duplicate per-opportunity dimension ASSIGNMENTS in a normalized junction
// table. Assignments already round-trip through
// OpportunityAssessment.Canonical (RMI-OMNIROADMAP-001)'s typed JSON, and
// portfolio-wide aggregation already has a home:
// assessment.ComputeDimensionDistribution operates directly on the
// in-memory []*OpportunityAssessment corpus. A redundant assignment table
// would duplicate that normative record for no query capability this repo
// actually needs yet.
type PortfolioDimension struct {
	ent.Schema
}

// Fields of the PortfolioDimension.
func (PortfolioDimension) Fields() []ent.Field {
	return []ent.Field{
		// id is "<dimensionId>:<version>" — multiple versions of the same
		// dimension ID coexist so a past assignment referencing an older
		// version still resolves (prism-roadmap TRD D5: assessments
		// reference a definition by ID+version, never a copy).
		field.String("id").MaxLen(320).NotEmpty(),
		field.String("dimension_id").MaxLen(191).NotEmpty(),
		field.String("version").MaxLen(64).NotEmpty(),
		field.String("name").MaxLen(255).NotEmpty(),
		// kind: "category" (0..1 selection, pie-chartable) | "tags" (0..N).
		field.String("kind").MaxLen(32).NotEmpty(),

		// built_in marks Kano/Market Investment Horizon — registered by
		// EnsureBuiltinDimensions — versus an organization-defined custom
		// dimension.
		field.Bool("built_in").Default(false),

		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

// Indexes of the PortfolioDimension.
func (PortfolioDimension) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("dimension_id"),
		index.Fields("dimension_id", "version").Unique(),
		index.Fields("built_in"),
	}
}
