package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ItemAugment holds locally-authored data layered on top of a synced Item —
// prioritization (MoSCoW, Kano, RICE), OKR links, and notes. It is keyed by
// the source system's stable human-facing reference (e.g. an Aha reference
// number like "MYPROJ-123") rather than the item row, so it survives
// re-syncs: sync overwrites the items table wholesale and never touches
// this table. Reads overlay augments onto items, with augment values
// winning over provider/fieldmap-derived ones.
type ItemAugment struct {
	ent.Schema
}

// Fields of the ItemAugment.
func (ItemAugment) Fields() []ent.Field {
	return []ent.Field{
		// id is "<provider>:<source_ref>".
		field.String("id").MaxLen(255).NotEmpty(),
		field.String("provider").MaxLen(64).NotEmpty(),
		// source_ref is the source system's reference (e.g. "MYPROJ-123");
		// for providers without human-facing refs, the source ID.
		field.String("source_ref").MaxLen(191).NotEmpty(),

		// moscow: must_have|should_have|could_have|wont_have; "" = unset.
		field.String("moscow").MaxLen(32).Optional(),
		// kano: must-be|performance|attractive|indifferent|reverse|
		// questionable; "" = unset.
		field.String("kano").MaxLen(32).Optional(),
		field.JSON("rice", map[string]float64{}).Optional(),

		// okr_refs links this item to OKR documents/objectives (e.g.
		// prism-roadmap objective IDs).
		field.JSON("okr_refs", []string{}).Optional(),
		field.Text("notes").Optional(),
		field.JSON("metadata", map[string]any{}).Optional(),

		field.Time("updated_at").Default(time.Now),
	}
}

// Indexes of the ItemAugment.
func (ItemAugment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "source_ref").Unique(),
	}
}
