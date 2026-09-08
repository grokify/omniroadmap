// Package analyticsdashboards ships a curated pack of DashForge dashboards
// over the omniroadmap analytics catalog (analyticscatalog, analyticsquery)
// -- the Splunk model: a generic analytics engine plus an application-
// specific "app" of prebuilt dashboards, rather than a hand-rolled UI.
// Every widget queries a dataset analyticscatalog.BuildFromStore actually
// describes and analyticsquery.Execute actually serves; no dashboard here
// references a field the catalog doesn't declare.
package analyticsdashboards

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/ProductBuildersHQ/compass-rice/rice"
	"github.com/grokify/prism-roadmap/assessment"
	"github.com/plexusone/dashforge/dashboardir"

	"github.com/grokify/omniroadmap/store"
)

// SourceID is the DashForge analytics source ID omniroadmap registers as
// (dashforgeconnector.ConnectorName / analyticscatalog.BuildFromStore).
const SourceID = "omniroadmap"

const (
	itemsDataset                  = "items"
	opportunityAssessmentsDataset = "opportunity_assessments"
	profileAssignmentsDataset     = "profile_assignments"
	compassEntityPrefix           = "compass_"
)

// DashboardPack bundles the curated dashboards with the saved questions
// their widgets reference -- both must be imported together into a
// dashforge-server; a dashboard alone is inert without its questions.
type DashboardPack struct {
	Dashboards []dashboardir.Dashboard     `json:"dashboards"`
	Questions  []dashboardir.SavedQuestion `json:"questions"`
}

// Build assembles the full curated pack: the two portfolio-wide
// prioritization views (compass-rice-all, moscow-compass-rice), one
// compass-profile-<slug> dashboard per COMPASS-RICE profile present in
// assessments, and portfolio-overview. generatedAt is stamped on every
// SavedQuestion rather than read from the clock internally, so a
// re-generated pack from the same inputs is reproducible.
func Build(assessments []assessment.OpportunityAssessment, generatedAt time.Time) DashboardPack {
	var pack DashboardPack

	add := func(d dashboardir.Dashboard, qs []dashboardir.SavedQuestion) {
		pack.Dashboards = append(pack.Dashboards, d)
		pack.Questions = append(pack.Questions, qs...)
	}

	add(compassRICEAllDashboard(generatedAt))
	add(moscowCompassRICEDashboard(generatedAt))
	for _, id := range compassProfileIDs(assessments) {
		add(compassProfileDashboard(id, assessments, generatedAt))
	}
	add(portfolioOverviewDashboard(generatedAt))

	return pack
}

// BuildFromStore reads the current assessment corpus from the store and
// builds the curated dashboard pack. generatedAt is stamped on every
// SavedQuestion -- pass time.Now() for a live build, or a fixed time for a
// reproducible one (e.g. in a test or a CLI --generated-at flag).
func BuildFromStore(ctx context.Context, s *store.DoltStore, generatedAt time.Time) (DashboardPack, error) {
	assessments, err := s.ListCurrentOpportunityAssessments(ctx)
	if err != nil {
		return DashboardPack{}, err
	}
	return Build(assessments, generatedAt), nil
}

func compassRICEAllDashboard(generatedAt time.Time) (dashboardir.Dashboard, []dashboardir.SavedQuestion) {
	ranked := question("compass-rice-all-ranked", "Ranked opportunities (COMPASS-RICE)",
		"Every computable opportunity, ranked purely by COMPASS-RICE score.",
		opportunityAssessmentsDataset,
		`SELECT spec_id, title, compass_profile_id, reach_band, score FROM opportunity_assessments WHERE computable = true ORDER BY score DESC LIMIT 200`,
		generatedAt)
	uncomputable := question("compass-rice-all-uncomputable", "Uncomputable opportunities",
		"Opportunities excluded from ranking, with the reason -- never silently dropped.",
		opportunityAssessmentsDataset,
		`SELECT spec_id, title, uncomputable_reason FROM opportunity_assessments WHERE computable = false LIMIT 200`,
		generatedAt)

	d := dashboard("compass-rice-all", "COMPASS-RICE: All Opportunities",
		"Every opportunity ranked by pure COMPASS-RICE score, plus what's still uncomputable and why. "+
			"See the per-profile dashboards for the raw evidence behind each score (compass-rice PRD D10: never the score alone).",
		[]dashboardir.Widget{
			widget("ranked", "Ranked by COMPASS-RICE Score", dashboardir.Position{X: 0, Y: 0, W: 12, H: 8}, ranked.ID, []dashboardir.TableColumn{
				column("spec_id", "Spec ID", ""),
				column("title", "Title", ""),
				column("compass_profile_id", "Profile", ""),
				column("reach_band", "Reach Band", ""),
				column("score", "Score", "number"),
			}),
			widget("uncomputable", "Uncomputable (excluded from ranking)", dashboardir.Position{X: 0, Y: 8, W: 12, H: 6}, uncomputable.ID, []dashboardir.TableColumn{
				column("spec_id", "Spec ID", ""),
				column("title", "Title", ""),
				column("uncomputable_reason", "Reason", ""),
			}),
		})
	return d, []dashboardir.SavedQuestion{ranked, uncomputable}
}

func moscowCompassRICEDashboard(generatedAt time.Time) (dashboardir.Dashboard, []dashboardir.SavedQuestion) {
	q := question("moscow-compass-rice-ranked", "MoSCoW + COMPASS-RICE ranking",
		"The canonical ranking view: MoSCoW tier first, COMPASS-RICE score descending within tier -- the same precedence RankingPolicy.Rank applies.",
		opportunityAssessmentsDataset,
		`SELECT spec_id, title, moscow, compass_profile_id, score, final_rank FROM opportunity_assessments WHERE computable = true ORDER BY moscow, score DESC LIMIT 200`,
		generatedAt)

	d := dashboard("moscow-compass-rice", "MoSCoW + COMPASS-RICE",
		"MoSCoW tier orders first, COMPASS-RICE score within tier. Final rank reflects any governance override -- compare against the raw score to see when and why they diverge.",
		[]dashboardir.Widget{
			widget("ranked", "Ranked by MoSCoW, then Score", dashboardir.Position{X: 0, Y: 0, W: 12, H: 10}, q.ID, []dashboardir.TableColumn{
				column("spec_id", "Spec ID", ""),
				column("title", "Title", ""),
				column("moscow", "MoSCoW", ""),
				column("compass_profile_id", "Profile", ""),
				column("score", "Score", "number"),
				column("final_rank", "Final Rank", "number"),
			}),
		})
	return d, []dashboardir.SavedQuestion{q}
}

// compassProfileDashboard builds the per-profile human-validation view:
// normalized columns beside the profile's raw evidence fields, discovered
// from the actual assessments present -- the same reasoning
// analyticscatalog.datasetForCompassProfile uses for its field list.
func compassProfileDashboard(id rice.ProfileID, assessments []assessment.OpportunityAssessment, generatedAt time.Time) (dashboardir.Dashboard, []dashboardir.SavedQuestion) {
	entityName := compassEntityName(id)
	slug := strings.TrimPrefix(entityName, compassEntityPrefix)

	columns := []dashboardir.TableColumn{
		column("spec_id", "Spec ID", ""),
		column("reach", "Reach", "number"),
		column("reach_band", "Reach Band", ""),
		column("impact", "Impact", "number"),
		column("confidence", "Confidence", "number"),
		column("effort_pd", "Effort (PD)", "number"),
		column("method", "Method", ""),
		column("score", "Score", "number"),
	}
	selectFields := []string{"spec_id", "reach", "reach_band", "impact", "confidence", "effort_pd", "method", "score"}
	for _, ref := range evidenceFieldsFor(id, assessments) {
		selectFields = append(selectFields, ref.queryName)
		columns = append(columns, column(ref.queryName, ref.label, ""))
	}

	q := question("compass-profile-"+slug+"-detail", "COMPASS: "+string(id),
		"Normalized score alongside the raw evidence that produced it -- human validation for "+string(id)+".",
		entityName,
		"SELECT "+strings.Join(selectFields, ", ")+" FROM "+entityName+" LIMIT 200",
		generatedAt)

	d := dashboard("compass-profile-"+slug, "COMPASS Profile: "+string(id),
		"Raw evidence beside the normalized band/method/score it produced, for "+string(id)+
			" -- validate a score against exactly what generated it (compass-rice PRD D10: never the score alone).",
		[]dashboardir.Widget{
			widget("detail", "Raw Evidence vs. Normalized Score", dashboardir.Position{X: 0, Y: 0, W: 12, H: 10}, q.ID, columns),
		})
	return d, []dashboardir.SavedQuestion{q}
}

func portfolioOverviewDashboard(generatedAt time.Time) (dashboardir.Dashboard, []dashboardir.SavedQuestion) {
	itemsQ := question("portfolio-overview-items", "Items by Provider, Kind, Status",
		"Synced item counts across every provider -- the general portfolio composition view.",
		itemsDataset,
		`SELECT provider, kind, status, COUNT(*) AS count FROM items GROUP BY provider, kind, status LIMIT 200`,
		generatedAt)
	freshnessQ := question("portfolio-overview-freshness", "Sync Freshness by Provider",
		"Most recent update timestamp synced per provider.",
		itemsDataset,
		`SELECT provider, MAX(updated_at) AS last_synced FROM items GROUP BY provider LIMIT 50`,
		generatedAt)
	scoringQ := question("portfolio-overview-scoring-coverage", "RICE Computable vs. Not",
		"How many current-cycle assessments have a computable COMPASS-RICE score versus how many are still awaiting assessment or PM confirmation.",
		opportunityAssessmentsDataset,
		`SELECT computable, COUNT(*) AS count FROM opportunity_assessments GROUP BY computable LIMIT 10`,
		generatedAt)
	profileStatusQ := question("portfolio-overview-profile-status", "Profile Assignment Status",
		"Two-phase COMPASS-RICE profile assignments by status: proposed, confirmed, or rejected.",
		profileAssignmentsDataset,
		`SELECT status, COUNT(*) AS count FROM profile_assignments GROUP BY status LIMIT 10`,
		generatedAt)

	d := dashboard("portfolio-overview", "Portfolio Overview",
		"Synced items by provider/kind/status, sync freshness, and COMPASS-RICE scoring coverage -- the general omniroadmap dashboard.",
		[]dashboardir.Widget{
			widget("items", "Items by Provider, Kind, Status", dashboardir.Position{X: 0, Y: 0, W: 6, H: 8}, itemsQ.ID, []dashboardir.TableColumn{
				column("provider", "Provider", ""),
				column("kind", "Kind", ""),
				column("status", "Status", ""),
				column("count", "Count", "number"),
			}),
			widget("freshness", "Sync Freshness by Provider", dashboardir.Position{X: 6, Y: 0, W: 6, H: 8}, freshnessQ.ID, []dashboardir.TableColumn{
				column("provider", "Provider", ""),
				column("last_synced", "Last Synced", "date"),
			}),
			widget("scoring", "RICE Computable vs. Not", dashboardir.Position{X: 0, Y: 8, W: 6, H: 6}, scoringQ.ID, []dashboardir.TableColumn{
				column("computable", "Computable", ""),
				column("count", "Count", "number"),
			}),
			widget("profile-status", "Profile Assignment Status", dashboardir.Position{X: 6, Y: 8, W: 6, H: 6}, profileStatusQ.ID, []dashboardir.TableColumn{
				column("status", "Status", ""),
				column("count", "Count", "number"),
			}),
		})
	return d, []dashboardir.SavedQuestion{itemsQ, freshnessQ, scoringQ, profileStatusQ}
}

// --- shared builders ---

func dashboard(id, title, description string, widgets []dashboardir.Widget) dashboardir.Dashboard {
	return dashboardir.Dashboard{
		ID:          id,
		Title:       title,
		Description: description,
		Layout:      dashboardir.Layout{Type: dashboardir.LayoutTypeGrid, Columns: 12, RowHeight: 40, Gap: 8},
		DataSources: []dashboardir.DataSource{{ID: SourceID, Name: "OmniRoadmap", Type: "derived"}},
		Widgets:     widgets,
	}
}

func widget(id, title string, pos dashboardir.Position, questionID string, columns []dashboardir.TableColumn) dashboardir.Widget {
	return dashboardir.Widget{
		ID:         id,
		Title:      title,
		Type:       "table",
		Position:   pos,
		QuestionID: questionID,
		Config:     mustTableConfig(columns),
	}
}

func column(field, header, format string) dashboardir.TableColumn {
	return dashboardir.TableColumn{Field: field, Header: header, Format: format}
}

func mustTableConfig(columns []dashboardir.TableColumn) json.RawMessage {
	data, err := json.Marshal(dashboardir.TableConfig{Columns: columns, Sortable: true})
	if err != nil {
		// TableConfig is a plain struct with no cyclic references or
		// unsupported types -- this can never fail.
		panic(fmt.Sprintf("analyticsdashboards: marshal table config: %v", err))
	}
	return data
}

func question(id, name, description, datasetID, query string, generatedAt time.Time) dashboardir.SavedQuestion {
	return dashboardir.SavedQuestion{
		ID:          id,
		Name:        name,
		Description: description,
		SourceID:    SourceID,
		DatasetID:   datasetID,
		Dialect:     "guardsql",
		Query:       query,
		CreatedAt:   generatedAt,
		UpdatedAt:   generatedAt,
	}
}

// --- COMPASS profile discovery and evidence-field naming ---
//
// Duplicated (not imported) from analyticscatalog/analyticsquery by this
// repo's existing convention (see e.g. customQueryName/customQueryField):
// each package independently derives the same field names from the same
// data, rather than sharing an internal package. evidenceQueryName and
// compassEntityName MUST stay byte-identical to their namesakes in
// analyticscatalog and analyticsquery -- a mismatch would mean a dashboard
// built here references a query name the executor doesn't recognize.

func compassProfileIDs(assessments []assessment.OpportunityAssessment) []rice.ProfileID {
	seen := map[rice.ProfileID]bool{}
	var ids []rice.ProfileID
	for _, a := range assessments {
		if a.Compass == nil || seen[a.Compass.ProfileID] {
			continue
		}
		seen[a.Compass.ProfileID] = true
		ids = append(ids, a.Compass.ProfileID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func compassEntityName(id rice.ProfileID) string {
	slug := strings.NewReplacer("/", "_", "-", "_").Replace(strings.ToLower(string(id)))
	return compassEntityPrefix + slug
}

type evidenceFieldRef struct {
	queryName string
	label     string
}

// evidenceFieldsFor discovers the raw evidence fields present for one
// profile's assessments, sorted by query name for deterministic output.
func evidenceFieldsFor(id rice.ProfileID, assessments []assessment.OpportunityAssessment) []evidenceFieldRef {
	seen := map[string]bool{}
	var out []evidenceFieldRef
	for _, a := range assessments {
		if a.Compass == nil || a.Compass.ProfileID != id || len(a.Compass.EvidenceJSON) == 0 {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal(a.Compass.EvidenceJSON, &decoded); err != nil {
			continue
		}
		keys := make([]string, 0, len(decoded))
		for k := range decoded {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			queryName := evidenceQueryName(key)
			if queryName == "" || seen[queryName] {
				continue
			}
			seen[queryName] = true
			out = append(out, evidenceFieldRef{queryName: queryName, label: humanizeKey(key)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].queryName < out[j].queryName })
	return out
}

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
