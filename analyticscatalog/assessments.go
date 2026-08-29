package analyticscatalog

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"

	"github.com/ProductBuildersHQ/compass-rice/rice"
	"github.com/grokify/prism-roadmap/assessment"
	"github.com/plexusone/dashforge/dashboardir"
)

// assessmentDatasets returns opportunity_assessments, profile_assignments,
// and one compass_<profile> dataset per COMPASS-RICE profile present in the
// corpus — the analytics surface for the three prioritization dashboard
// views (all-items, MoSCoW+COMPASS-RICE, per-profile) plus the two-phase
// assignment lifecycle.
func assessmentDatasets(assessments []assessment.OpportunityAssessment, assignments []assessment.ProfileAssignment, ranks map[string]assessment.OpportunityRank) []dashboardir.AnalyticsDataset {
	datasets := []dashboardir.AnalyticsDataset{
		datasetForAssessments(assessments, ranks),
		datasetForProfileAssignments(assignments),
	}
	return append(datasets, datasetsForCompassProfiles(assessments)...)
}

// assessmentRow is the per-assessment data the opportunity_assessments
// dataset's field coverage stats are computed from — resolved once per
// assessment so every field's presence check reads the same values.
type assessmentRow struct {
	specID           string
	title            string
	moscow           string
	hasCompass       bool
	compassProfileID string
	reachBand        string
	cycleNumber      int
	rank             *assessment.OpportunityRank
	rice             assessment.RICEScoreResult
	needsHumanReview bool
	humanReviewed    bool
}

func toAssessmentRow(a assessment.OpportunityAssessment, ranks map[string]assessment.OpportunityRank) assessmentRow {
	row := assessmentRow{
		specID:      a.Opportunity.SpecID,
		title:       a.Title,
		moscow:      a.MoSCoW().String(),
		cycleNumber: a.Cycle.Number,
	}
	switch {
	case a.Compass != nil:
		row.hasCompass = true
		row.compassProfileID = string(a.Compass.ProfileID)
		row.reachBand = string(a.Compass.Normalized.ReachBand)
		row.needsHumanReview = a.Compass.NeedsHumanReview
		row.humanReviewed = a.Compass.HumanReview != nil
		row.rice = assessment.ResolveCompassRICE(a.Compass)
	case a.RICE != nil:
		row.rice = assessment.ComputeRICE(*a.RICE)
	}
	if r, ok := ranks[a.ID]; ok {
		row.rank = &r
	}
	return row
}

// datasetForAssessments describes the current-cycle assessment corpus:
// compass-first RICE resolution (matching ToRankInput's own precedence),
// PM-review state, and rank — the "all items" and "MoSCoW+COMPASS-RICE"
// dashboard views query this dataset.
func datasetForAssessments(assessments []assessment.OpportunityAssessment, ranks map[string]assessment.OpportunityRank) dashboardir.AnalyticsDataset {
	rows := make([]assessmentRow, len(assessments))
	for i, a := range assessments {
		rows[i] = toAssessmentRow(a, ranks)
	}
	total := len(rows)

	defs := []struct {
		queryName, name, typ, role string
		present                    func(r assessmentRow) bool
	}{
		{"spec_id", "Spec ID", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(r assessmentRow) bool { return r.specID != "" }},
		{"title", "Title", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(r assessmentRow) bool { return r.title != "" }},
		{"moscow", "MoSCoW", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(r assessmentRow) bool { return r.moscow != "" }},
		{"compass_profile_id", "COMPASS Profile", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(r assessmentRow) bool { return r.hasCompass }},
		{"reach_band", "Reach Band", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(r assessmentRow) bool { return r.hasCompass }},
		{"score", "RICE Score", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldRoleMeasure, func(r assessmentRow) bool { return r.rice.Computable }},
		{"computable", "Computable", dashboardir.AnalyticsFieldTypeBool, dashboardir.AnalyticsFieldRoleDimension, func(r assessmentRow) bool { return true }},
		{"uncomputable_reason", "Uncomputable Reason", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(r assessmentRow) bool { return r.rice.Reason != "" }},
		{"needs_human_review", "Needs Human Review", dashboardir.AnalyticsFieldTypeBool, dashboardir.AnalyticsFieldRoleDimension, func(r assessmentRow) bool { return true }},
		{"human_reviewed", "Human Reviewed", dashboardir.AnalyticsFieldTypeBool, dashboardir.AnalyticsFieldRoleDimension, func(r assessmentRow) bool { return true }},
		{"cycle_number", "Cycle Number", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldRoleMeasure, func(r assessmentRow) bool { return true }},
		{"calculated_rank", "Calculated Rank", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldRoleMeasure, func(r assessmentRow) bool { return r.rank != nil }},
		{"final_rank", "Final Rank", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldRoleMeasure, func(r assessmentRow) bool { return r.rank != nil }},
	}

	fields := make([]dashboardir.AnalyticsField, 0, len(defs))
	for _, d := range defs {
		count := 0
		for _, r := range rows {
			if d.present(r) {
				count++
			}
		}
		fields = append(fields, standardField(d.queryName, d.name, d.typ, d.role, count, total))
	}

	return dashboardir.AnalyticsDataset{
		ID:          "opportunity_assessments",
		Name:        "Opportunity Assessments",
		QueryName:   "opportunity_assessments",
		Description: "Current-cycle assessments with compass-first RICE resolution and PM-review/rank state.",
		Fields:      fields,
	}
}

// datasetForProfileAssignments describes the two-phase (LLM-proposed,
// PM-confirmed) primary investment thesis for each opportunity.
func datasetForProfileAssignments(assignments []assessment.ProfileAssignment) dashboardir.AnalyticsDataset {
	total := len(assignments)

	defs := []struct {
		queryName, name, typ, role string
		present                    func(p assessment.ProfileAssignment) bool
	}{
		{"spec_id", "Spec ID", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(p assessment.ProfileAssignment) bool { return p.SpecID != "" }},
		{"profile_id", "Profile ID", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(p assessment.ProfileAssignment) bool { return p.ProfileID != "" }},
		{"status", "Status", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(p assessment.ProfileAssignment) bool { return p.Status != "" }},
		{"proposed_by", "Proposed By", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(p assessment.ProfileAssignment) bool { return p.ProposedBy != "" }},
		{"confirmed_by", "Confirmed By", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(p assessment.ProfileAssignment) bool { return p.ConfirmedBy != "" }},
		{"rationale", "Rationale", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, func(p assessment.ProfileAssignment) bool { return p.Rationale != "" }},
	}

	fields := make([]dashboardir.AnalyticsField, 0, len(defs))
	for _, d := range defs {
		count := 0
		for _, p := range assignments {
			if d.present(p) {
				count++
			}
		}
		fields = append(fields, standardField(d.queryName, d.name, d.typ, d.role, count, total))
	}

	return dashboardir.AnalyticsDataset{
		ID:          "profile_assignments",
		Name:        "Profile Assignments",
		QueryName:   "profile_assignments",
		Description: "Two-phase COMPASS-RICE profile assignments: proposed, confirmed, or rejected.",
		Fields:      fields,
	}
}

// datasetsForCompassProfiles returns one dataset per COMPASS-RICE ProfileID
// present in assessments (e.g. compass_customer_b2b_v1), each pairing
// normalized columns with the profile's raw evidence fields so a PM can
// validate a normalized score directly against what produced it.
func datasetsForCompassProfiles(assessments []assessment.OpportunityAssessment) []dashboardir.AnalyticsDataset {
	byProfile := map[rice.ProfileID][]assessment.OpportunityAssessment{}
	for _, a := range assessments {
		if a.Compass == nil {
			continue
		}
		byProfile[a.Compass.ProfileID] = append(byProfile[a.Compass.ProfileID], a)
	}

	ids := make([]string, 0, len(byProfile))
	for id := range byProfile {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	datasets := make([]dashboardir.AnalyticsDataset, 0, len(ids))
	for _, id := range ids {
		datasets = append(datasets, datasetForCompassProfile(rice.ProfileID(id), byProfile[rice.ProfileID(id)]))
	}
	return datasets
}

func compassProfileDatasetID(id rice.ProfileID) string {
	slug := strings.NewReplacer("/", "_", "-", "_").Replace(string(id))
	return "compass_" + slug
}

func datasetForCompassProfile(id rice.ProfileID, assessments []assessment.OpportunityAssessment) dashboardir.AnalyticsDataset {
	total := len(assessments)

	// Normalized columns: always present for every row here by
	// construction (every assessment in this slice has this exact
	// ProfileID's Normalized result).
	fields := []dashboardir.AnalyticsField{
		standardField("spec_id", "Spec ID", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, total, total),
		standardField("reach", "Reach", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldRoleMeasure, total, total),
		standardField("reach_band", "Reach Band", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, total, total),
		standardField("impact", "Impact", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldRoleMeasure, total, total),
		standardField("confidence", "Confidence", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldRoleMeasure, total, total),
		standardField("effort_pd", "Effort (PD)", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldRoleMeasure, total, total),
		standardField("method", "Normalization Method", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldRoleDimension, total, total),
	}
	scoreCount := 0
	for _, a := range assessments {
		if _, err := a.Compass.Normalized.Score(); err == nil {
			scoreCount++
		}
	}
	fields = append(fields, standardField("score", "RICE Score", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldRoleMeasure, scoreCount, total))

	// Raw evidence fields, flattened from EvidenceJSON -- the profile's
	// own evidence schema, dynamically discovered the same way item
	// custom fields are (customStats), so a new profile or evidence-model
	// version needs no code change here to show up.
	evidence := newCustomStats(total)
	for i := range assessments {
		flattenEvidenceJSON(evidence, assessments[i].Compass.EvidenceJSON)
	}
	fields = append(fields, evidence.fields()...)

	return dashboardir.AnalyticsDataset{
		ID:          compassProfileDatasetID(id),
		Name:        "COMPASS: " + string(id),
		QueryName:   compassProfileDatasetID(id),
		Description: "Normalized score alongside the raw evidence that produced it, for " + string(id) + " — human validation of one profile's assessments.",
		Fields:      fields,
	}
}

// flattenEvidenceJSON records one field per top-level key of a profile's
// raw evidence document into stats -- the same dynamic-field-discovery
// mechanism customStats.add uses for provider custom fields, applied to a
// compass-rice Evidence struct's JSON shape instead. Query names are
// prefixed "evidence." to keep them visually and namespace-distinct from
// the normalized columns in the same dataset.
func flattenEvidenceJSON(stats *customStats, evidenceJSON json.RawMessage) {
	if len(evidenceJSON) == 0 {
		return
	}
	var decoded map[string]any
	if err := json.Unmarshal(evidenceJSON, &decoded); err != nil {
		return
	}

	keys := make([]string, 0, len(decoded))
	for k := range decoded {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := decoded[key]
		queryName := "evidence." + key
		stat := stats.byName[queryName]
		if stat == nil {
			typ := inferType(value)
			role := dashboardir.AnalyticsFieldRoleDimension
			if typ == dashboardir.AnalyticsFieldTypeNumber {
				role = dashboardir.AnalyticsFieldRoleMeasure
			}
			stat = &dashboardir.AnalyticsField{
				ID: queryName, Name: humanizeKey(key), QueryName: queryName, Type: typ,
				Source: dashboardir.AnalyticsFieldSourceStandard, Role: role,
				Selectable: true, Filterable: true, Sortable: true, Nullable: true,
			}
			stats.byName[queryName] = stat
			stats.values[queryName] = map[string]struct{}{}
		}
		stat.Count++
		if s := sampleString(value); s != "" && len(stats.values[queryName]) < 8 {
			stats.values[queryName][s] = struct{}{}
		}
	}
}

// humanizeKey turns a lowerCamelCase evidence field key (e.g.
// "eligibleArr") into a display label ("Eligible Arr").
func humanizeKey(key string) string {
	var b strings.Builder
	for i, r := range key {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteRune(' ')
		}
		if i == 0 {
			r = unicode.ToUpper(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// standardField builds a fixed-shape AnalyticsField (as opposed to a
// dynamically-discovered one like customStats produces) with Count/Coverage
// computed from real rows, matching standardFields' convention for items.
func standardField(queryName, name, typ, role string, count, total int) dashboardir.AnalyticsField {
	coverage := 0.0
	if total > 0 {
		coverage = float64(count) / float64(total)
	}
	return dashboardir.AnalyticsField{
		ID:         queryName,
		Name:       name,
		QueryName:  queryName,
		Type:       typ,
		Source:     dashboardir.AnalyticsFieldSourceStandard,
		Role:       role,
		Selectable: true,
		Filterable: true,
		Sortable:   true,
		Nullable:   count < total,
		Count:      count,
		Coverage:   coverage,
	}
}
