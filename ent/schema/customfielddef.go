package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CustomFieldDef holds a custom field definition (schema/metadata, not
// per-record values) synced from a provider.
type CustomFieldDef struct {
	ent.Schema
}

// Fields of the CustomFieldDef.
func (CustomFieldDef) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(255).NotEmpty(), // "<provider>:<key>"
		field.String("provider").MaxLen(64).NotEmpty(),
		field.String("key").MaxLen(191).NotEmpty(),
		field.String("name").MaxLen(255).Optional(),
		field.String("field_type").MaxLen(64).Optional(),
		field.JSON("kinds", []string{}).Optional(),
		field.JSON("metadata", map[string]any{}).Optional(),
		field.Time("synced_at").Default(time.Now),
	}
}

// Indexes of the CustomFieldDef.
func (CustomFieldDef) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "key").Unique(),
	}
}
