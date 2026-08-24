package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/grokify/prism-roadmap/assessment"
)

// DimensionOption holds one selectable option within a PortfolioDimension
// (e.g. Kano's "must_be", or a custom dimension's "ai"), including the
// judge Questions used to determine whether an opportunity belongs to it.
// Rows reference their parent PortfolioDimension by a plain string column
// (DimensionRowID), matching this repo's existing flat-table style (no
// Ent edges are used elsewhere in ent/schema).
type DimensionOption struct {
	ent.Schema
}

// Fields of the DimensionOption.
func (DimensionOption) Fields() []ent.Field {
	return []ent.Field{
		// id is "<PortfolioDimension.id>:<optionId>".
		field.String("id").MaxLen(384).NotEmpty(),
		// dimension_row_id is the parent PortfolioDimension.ID.
		field.String("dimension_row_id").MaxLen(320).NotEmpty(),
		field.String("option_id").MaxLen(191).NotEmpty(),
		field.String("label").MaxLen(255).NotEmpty(),

		// questions is the option's assessment.DimensionQuestion list —
		// small, always read/written as a unit with the option, not
		// queried independently, so it's stored as typed JSON rather than
		// its own table.
		field.JSON("questions", []assessment.DimensionQuestion{}).Optional(),

		// sequence preserves the definition's option order for rendering.
		field.Int("sequence").Default(0),
	}
}

// Indexes of the DimensionOption.
func (DimensionOption) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("dimension_row_id"),
		index.Fields("dimension_row_id", "option_id").Unique(),
	}
}
