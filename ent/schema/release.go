package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Release holds a canonical release/milestone synced from a provider.
type Release struct {
	ent.Schema
}

// Fields of the Release.
func (Release) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(255).NotEmpty(),
		field.String("provider").MaxLen(64).NotEmpty(),
		field.String("source_id").MaxLen(191).NotEmpty(),
		field.String("source_ref").MaxLen(191).Optional(),
		field.String("source_url").MaxLen(512).Optional(),

		field.String("name").MaxLen(512).Optional(),

		field.Time("start_date").Optional().Nillable(),
		field.Time("release_date").Optional().Nillable(),
		field.Bool("released").Default(false),

		field.String("status_id").MaxLen(191).Optional(),
		field.String("status_name").MaxLen(191).Optional(),
		field.String("status_category").MaxLen(32).Optional(),
		field.Float("progress").Optional().Nillable(),

		field.JSON("metadata", map[string]any{}).Optional(),

		field.Time("synced_at").Default(time.Now),
	}
}

// Indexes of the Release.
func (Release) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider"),
		index.Fields("released"),
	}
}
