package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SyncMeta tracks the last sync time and record count per (provider, kind),
// mirroring aha-studio's sync_meta pattern.
type SyncMeta struct {
	ent.Schema
}

// Annotations of the SyncMeta.
func (SyncMeta) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "sync_meta"},
	}
}

// Fields of the SyncMeta.
func (SyncMeta) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").MaxLen(64).NotEmpty(),
		field.String("kind").MaxLen(32).NotEmpty(),
		field.Time("last_sync"),
		field.Int("record_count").Default(0),
	}
}

// Indexes of the SyncMeta.
func (SyncMeta) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "kind").Unique(),
	}
}
