package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Item holds the canonical, tool-agnostic roadmap item (feature, epic,
// initiative, objective, key result) synced from a provider. Common fields
// are flattened into columns for indexing/filtering; the long tail
// (custom fields, RICE, provider metadata) is stored as JSON.
type Item struct {
	ent.Schema
}

// Fields of the Item.
func (Item) Fields() []ent.Field {
	return []ent.Field{
		// id is the canonical ID: "<provider>:<source id>".
		field.String("id").MaxLen(255).NotEmpty(),
		field.String("provider").MaxLen(64).NotEmpty(),
		field.String("source_id").MaxLen(191).NotEmpty(),
		field.String("source_ref").MaxLen(191).Optional(),
		field.String("source_url").MaxLen(512).Optional(),
		field.String("workspace_ref").MaxLen(191).Optional(),

		field.String("kind").MaxLen(32).NotEmpty(),
		field.String("name").MaxLen(512).Optional(),
		field.Text("description").Optional(),

		field.String("status_id").MaxLen(191).Optional(),
		field.String("status_name").MaxLen(191).Optional(),
		field.String("status_category").MaxLen(32).Optional(),
		field.Bool("status_complete").Default(false),
		field.Float("progress").Optional().Nillable(),

		field.String("parent_id").MaxLen(255).Optional(),
		field.String("release_id").MaxLen(255).Optional(),

		field.Time("start_date").Optional().Nillable(),
		field.Time("due_date").Optional().Nillable(),
		field.Time("created_at").Optional().Nillable(),
		field.Time("updated_at").Optional().Nillable(),

		field.String("owner_id").MaxLen(191).Optional(),
		field.String("owner_name").MaxLen(191).Optional(),
		field.String("owner_email").MaxLen(191).Optional(),

		// moscow: must_have|should_have|could_have|wont_have; "" = unset.
		field.String("moscow").MaxLen(32).Optional(),
		// kano: must-be|performance|attractive|indifferent|reverse|
		// questionable; "" = unset.
		field.String("kano").MaxLen(32).Optional(),
		field.JSON("rice", map[string]float64{}).Optional(),

		field.JSON("tags", []string{}).Optional(),
		field.JSON("custom_fields", []map[string]any{}).Optional(),
		field.JSON("metadata", map[string]any{}).Optional(),

		field.Time("synced_at").Default(time.Now),
	}
}

// Indexes of the Item.
func (Item) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider"),
		index.Fields("kind"),
		index.Fields("provider", "kind"),
		index.Fields("provider", "source_ref"),
		index.Fields("provider", "workspace_ref"),
		index.Fields("status_category"),
		index.Fields("moscow"),
		index.Fields("release_id"),
	}
}
