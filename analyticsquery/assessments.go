package analyticsquery

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/ProductBuildersHQ/compass-rice/rice"
	"github.com/grokify/guardsql"
	"github.com/grokify/prism-roadmap/assessment"
)

const (
	assessmentEntityName        = "opportunity_assessments"
	profileAssignmentEntityName = "profile_assignments"
	compassEntityPrefix         = "compass_"
)

// assessmentEntities builds the guardsql schema entities for the assessment
// family (opportunity_assessments, profile_assignments, and one
// compass_<profile> per ProfileID present) -- mirroring exactly the field
// set analyticscatalog.BuildFromStore describes for each dataset, so a
// dashboard can only select what the catalog told it exists.
func assessmentEntities(assessments []assessment.OpportunityAssessment) map[string]guardsql.Entity {
	entities := map[string]guardsql.Entity{
		assessmentEntityName:        {Name: assessmentEntityName, Fields: assessmentFields()},
		profileAssignmentEntityName: {Name: profileAssignmentEntityName, Fields: profileAssignmentFields()},
	}
	for entityName, id := range compassEntityMap(assessments) {
		entities[entityName] = guardsql.Entity{Name: entityName, Fields: compassProfileFields(id, assessments)}
	}
	return entities
}

func assessmentFields() map[string]guardsql.Field {
	fields := map[string]guardsql.Field{}
	add := func(name string, typ guardsql.FieldType) {
		fields[name] = guardsql.Field{Name: name, Type: typ, Selectable: true, Filterable: true, Sortable: true}
	}
	add("spec_id", guardsql.FieldString)
	add("title", guardsql.FieldString)
	add("moscow", guardsql.FieldString)
	add("compass_profile_id", guardsql.FieldString)
	add("reach_band", guardsql.FieldString)
	add("score", guardsql.FieldNumber)
	add("computable", guardsql.FieldBool)
	add("uncomputable_reason", guardsql.FieldString)
	add("needs_human_review", guardsql.FieldBool)
	add("human_reviewed", guardsql.FieldBool)
	add("cycle_number", guardsql.FieldNumber)
	add("calculated_rank", guardsql.FieldNumber)
	add("final_rank", guardsql.FieldNumber)
	return fields
}

func profileAssignmentFields() map[string]guardsql.Field {
	fields := map[string]guardsql.Field{}
	add := func(name string) {
		fields[name] = guardsql.Field{Name: name, Type: guardsql.FieldString, Selectable: true, Filterable: true, Sortable: true}
	}
	add("spec_id")
	add("profile_id")
	add("status")
	add("proposed_by")
	add("confirmed_by")
	add("rationale")
	return fields
}

// compassProfileFields describes one profile-scoped entity's fields:
// normalized columns, always present, plus raw evidence fields dynamically
// discovered from that profile's assessments -- the same
// discover-from-real-data approach items' custom fields use.
func compassProfileFields(id rice.ProfileID, assessments []assessment.OpportunityAssessment) map[string]guardsql.Field {
	fields := map[string]guardsql.Field{}
	add := func(name string, typ guardsql.FieldType) {
		fields[name] = guardsql.Field{Name: name, Type: typ, Selectable: true, Filterable: true, Sortable: true}
	}
	add("spec_id", guardsql.FieldString)
	add("reach", guardsql.FieldNumber)
	add("reach_band", guardsql.FieldString)
	add("impact", guardsql.FieldNumber)
	add("confidence", guardsql.FieldNumber)
	add("effort_pd", guardsql.FieldNumber)
	add("method", guardsql.FieldString)
	add("score", guardsql.FieldNumber)

	for _, a := range assessments {
		if a.Compass == nil || a.Compass.ProfileID != id {
			continue
		}
		for key, value := range decodeEvidence(a.Compass.EvidenceJSON) {
			queryName := evidenceQueryName(key)
			if queryName == "" {
				continue
			}
			if _, exists := fields[queryName]; !exists {
				fields[queryName] = guardsql.Field{Name: queryName, Type: inferFieldType(value), Selectable: true, Filterable: true, Sortable: true}
			}
		}
	}
	return fields
}

// compassEntityMap returns the entity-name -> ProfileID mapping for every
// COMPASS-RICE profile present in assessments. Slugification (e.g.
// "customer/b2b/v1" -> "compass_customer_b2b_v1") is lossy to invert, so
// this map is the single source of truth both schema-building and
// row-dispatch use to go from entity name back to ProfileID.
func compassEntityMap(assessments []assessment.OpportunityAssessment) map[string]rice.ProfileID {
	seen := map[rice.ProfileID]bool{}
	out := map[string]rice.ProfileID{}
	for _, a := range assessments {
		if a.Compass == nil || seen[a.Compass.ProfileID] {
			continue
		}
		seen[a.Compass.ProfileID] = true
		out[compassEntityName(a.Compass.ProfileID)] = a.Compass.ProfileID
	}
	return out
}

func compassEntityName(id rice.ProfileID) string {
	slug := strings.NewReplacer("/", "_", "-", "_").Replace(strings.ToLower(string(id)))
	return compassEntityPrefix + slug
}

// evidenceQueryName turns a raw evidence JSON key into a lowercase,
// "evidence."-prefixed query name -- the same transformation
// customQueryField uses for the "custom." prefix. Must match
// analyticscatalog's identically-named function exactly: guardsql.Schema.
// Normalize() lowercases field names internally, so a mismatched query name
// between the catalog and the query executor would silently fail to select.
func evidenceQueryName(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '_' || r == '-' || r == ' ' || r == '.':
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return ""
	}
	return "evidence." + out
}

func decodeEvidence(evidenceJSON json.RawMessage) map[string]any {
	if len(evidenceJSON) == 0 {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(evidenceJSON, &decoded); err != nil {
		return nil
	}
	return decoded
}

// assessmentRows builds opportunity_assessments rows: compass-first RICE
// resolution (matching ToRankInput's own precedence) and rank, joined from
// ranks (the latest compiled ReportDataset, keyed by AssessmentID).
func assessmentRows(assessments []assessment.OpportunityAssessment, ranks map[string]assessment.OpportunityRank) []guardsql.Row {
	rows := make([]guardsql.Row, 0, len(assessments))
	for _, a := range assessments {
		row := guardsql.Row{
			"spec_id":            a.Opportunity.SpecID,
			"title":              a.Title,
			"moscow":             a.MoSCoW().String(),
			"cycle_number":       float64(a.Cycle.Number),
			"computable":         false,
			"needs_human_review": false,
			"human_reviewed":     false,
		}

		var result assessment.RICEScoreResult
		switch {
		case a.Compass != nil:
			row["compass_profile_id"] = string(a.Compass.ProfileID)
			row["reach_band"] = string(a.Compass.Normalized.ReachBand)
			row["needs_human_review"] = a.Compass.NeedsHumanReview
			row["human_reviewed"] = a.Compass.HumanReview != nil
			result = assessment.ResolveCompassRICE(a.Compass)
		case a.RICE != nil:
			result = assessment.ComputeRICE(*a.RICE)
		}
		row["computable"] = result.Computable
		if result.Computable {
			row["score"] = result.Score
		}
		if result.Reason != "" {
			row["uncomputable_reason"] = result.Reason
		}

		if r, ok := ranks[a.ID]; ok {
			row["calculated_rank"] = float64(r.CalculatedRank)
			row["final_rank"] = float64(r.FinalRank)
		}
		rows = append(rows, row)
	}
	return rows
}

func profileAssignmentRows(assignments []assessment.ProfileAssignment) []guardsql.Row {
	rows := make([]guardsql.Row, 0, len(assignments))
	for _, p := range assignments {
		rows = append(rows, guardsql.Row{
			"spec_id":      p.SpecID,
			"profile_id":   string(p.ProfileID),
			"status":       string(p.Status),
			"proposed_by":  p.ProposedBy,
			"confirmed_by": p.ConfirmedBy,
			"rationale":    p.Rationale,
		})
	}
	return rows
}

// compassProfileRows builds rows for one profile-scoped entity: normalized
// columns beside the profile's raw evidence fields, flattened from
// EvidenceJSON -- the human-validation view pairing a normalized score with
// exactly what produced it.
func compassProfileRows(id rice.ProfileID, assessments []assessment.OpportunityAssessment) []guardsql.Row {
	var rows []guardsql.Row
	for _, a := range assessments {
		if a.Compass == nil || a.Compass.ProfileID != id {
			continue
		}
		n := a.Compass.Normalized
		row := guardsql.Row{
			"spec_id":    a.Opportunity.SpecID,
			"reach":      n.Reach,
			"reach_band": string(n.ReachBand),
			"impact":     float64(n.Impact),
			"confidence": float64(n.Confidence),
			"effort_pd":  n.EffortPD,
			"method":     n.Method,
		}
		if score, err := n.Score(); err == nil {
			row["score"] = score
		}
		for key, value := range decodeEvidence(a.Compass.EvidenceJSON) {
			if queryName := evidenceQueryName(key); queryName != "" {
				row[queryName] = queryValue(value)
			}
		}
		rows = append(rows, row)
	}
	return rows
}
