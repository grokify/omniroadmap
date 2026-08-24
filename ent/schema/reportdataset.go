package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/grokify/prism-roadmap/assessment"
)

// ReportDataset holds one compile run's output
// (github.com/grokify/prism-roadmap/assessment.ReportDataset,
// RMI-PRISMROADMAP-010) as the normative record. Every compile produces a
// NEW row — datasets are never mutated in place, mirroring
// OpportunityAssessment's own cycle-history discipline. Status tracks
// where a dataset sits in the PM review workflow: "draft" (just compiled,
// awaiting review) or "final" (reviewed and rank-materialized —
// RMI-OMNIROADMAP-006).
type ReportDataset struct {
	ent.Schema
}

// Fields of the ReportDataset.
func (ReportDataset) Fields() []ent.Field {
	return []ent.Field{
		// id is a compile-run identifier, caller-assigned (e.g. a
		// timestamp-based string) so it's stable and sortable without a
		// database-generated sequence.
		field.String("id").MaxLen(64).NotEmpty(),

		field.Time("generated_at"),
		field.String("ranking_policy_id").MaxLen(191).NotEmpty(),
		field.String("ranking_policy_version").MaxLen(64).NotEmpty(),

		// status: "draft" | "final".
		field.String("status").MaxLen(32).Default("draft"),

		// canonical is the full assessment.ReportDataset, typed directly.
		field.JSON("canonical", assessment.ReportDataset{}),

		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

// Indexes of the ReportDataset.
func (ReportDataset) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("generated_at"),
	}
}
