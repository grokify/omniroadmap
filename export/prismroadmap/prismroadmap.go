// Package prismroadmap converts canonical omniroadmap items into
// prism-roadmap types (github.com/grokify/prism-roadmap), so PM data
// ingested from Aha/ProductBoard/JPD can feed prism-roadmap's
// prioritization tooling, CLI, MCP server, and visualization pipeline.
//
// Modeled on prism-maturity/export/roadmap.go's converter shape: small
// per-field convert functions with one exported entry point per target
// type. The conversion is intentionally lossy-but-safe: fields with no
// prism-roadmap equivalent (custom fields, provenance beyond the source
// URL) are dropped, and prioritization fields are carried over only when
// actually present — MoSCoW is optional on rmi.RoadmapItem as of
// prism-roadmap v0.17.0, so unprioritized imported items stay honestly
// unprioritized rather than defaulting.
package prismroadmap

import (
	"fmt"
	"time"

	"github.com/grokify/prism-roadmap/prioritization"
	"github.com/grokify/prism-roadmap/rmi"

	"github.com/grokify/omniroadmap-core/provider"
)

// ToRoadmapItemSet converts canonical Items into an rmi.RoadmapItemSet.
// Every returned item passes rmi's own Validate; items that fail
// conversion (no ID/Name) are skipped and reported in the error joined
// across all failures, alongside the successfully converted set.
func ToRoadmapItemSet(items []provider.Item) (*rmi.RoadmapItemSet, error) {
	set := rmi.NewRoadmapItemSet()
	var errs []error
	for i := range items {
		item, err := ToRoadmapItem(&items[i])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		set.Add(*item)
	}
	if len(errs) > 0 {
		return set, fmt.Errorf("prismroadmap: %d item(s) skipped: %w", len(errs), joinErrors(errs))
	}
	return set, nil
}

// ToRoadmapItem converts one canonical Item to an rmi.RoadmapItem.
func ToRoadmapItem(item *provider.Item) (*rmi.RoadmapItem, error) {
	if item.ID == "" || item.Name == "" {
		return nil, fmt.Errorf("item %q: id and name are required", item.ID)
	}

	out := &rmi.RoadmapItem{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		MoSCoW:      prioritization.MoSCoWPriority(item.MoSCoW),
		RICE:        convertRICE(item),
		StartDate:   formatDate(item.StartDate),
		DueDate:     formatDate(item.DueDate),
		Status:      convertStatus(item.Status),
		Owner:       convertOwner(item.Owner),
		Tags:        item.Tags,
		Notes:       convertNotes(item),
	}
	if item.CreatedAt != nil {
		out.CreatedAt = *item.CreatedAt
	}
	if item.UpdatedAt != nil {
		out.UpdatedAt = *item.UpdatedAt
	}
	if item.Progress != nil {
		progress := int(*item.Progress)
		out.Progress = &progress
	}

	if err := out.Validate(); err != nil {
		return nil, fmt.Errorf("item %q: converted item invalid: %w", item.ID, err)
	}
	return out, nil
}

// convertRICE maps canonical raw-float RICE inputs onto
// prioritization.RICEScore, translating numeric impact/confidence
// multipliers to prism-roadmap's nearest enum levels. Returns nil when the
// item has no RICE data.
func convertRICE(item *provider.Item) *prioritization.RICEScore {
	r := item.RICE
	if r == nil {
		return nil
	}
	score := &prioritization.RICEScore{
		FeatureID:   item.ID,
		FeatureName: item.Name,
	}
	if r.Reach != nil {
		score.Reach = int(*r.Reach)
	}
	if r.Impact != nil {
		score.Impact = nearestImpact(*r.Impact)
	}
	if r.Confidence != nil {
		score.Confidence = nearestConfidence(*r.Confidence)
	}
	if r.Effort != nil {
		score.Effort = *r.Effort
	}
	if r.Score != nil {
		score.Score = *r.Score
	} else if score.Reach > 0 && score.Impact != "" && score.Confidence != "" && score.Effort > 0 {
		score.Calculate()
	}
	return score
}

// nearestImpact maps a raw impact multiplier to the closest
// prism-roadmap ImpactLevel (massive=3.0, high=2.0, medium=1.0, low=0.5,
// minimal=0.25).
func nearestImpact(v float64) prioritization.ImpactLevel {
	levels := []struct {
		level      prioritization.ImpactLevel
		multiplier float64
	}{
		{prioritization.ImpactMassive, 3.0},
		{prioritization.ImpactHigh, 2.0},
		{prioritization.ImpactMedium, 1.0},
		{prioritization.ImpactLow, 0.5},
		{prioritization.ImpactMinimal, 0.25},
	}
	return nearestLevel(v, levels)
}

// nearestConfidence maps a raw confidence multiplier (0-1, or 0-100 as a
// percentage) to the closest ConfidenceLevel (high=1.0, medium=0.8,
// low=0.5).
func nearestConfidence(v float64) prioritization.ConfidenceLevel {
	if v > 1 {
		v /= 100 // tolerate percentage-style inputs (e.g. 80 -> 0.8)
	}
	levels := []struct {
		level      prioritization.ConfidenceLevel
		multiplier float64
	}{
		{prioritization.ConfidenceHigh, 1.0},
		{prioritization.ConfidenceMedium, 0.8},
		{prioritization.ConfidenceLow, 0.5},
	}
	return nearestLevel(v, levels)
}

func nearestLevel[T ~string](v float64, levels []struct {
	level      T
	multiplier float64
}) T {
	best := levels[0].level
	bestDist := dist(v, levels[0].multiplier)
	for _, l := range levels[1:] {
		if d := dist(v, l.multiplier); d < bestDist {
			best, bestDist = l.level, d
		}
	}
	return best
}

func dist(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}

// convertStatus maps a canonical StatusCategory to an RMIStatus. Unset or
// unrecognized statuses default to planned.
func convertStatus(s *provider.Status) rmi.RMIStatus {
	if s == nil {
		return rmi.RMIStatusPlanned
	}
	switch s.Category {
	case provider.StatusCategoryTodo:
		return rmi.RMIStatusPlanned
	case provider.StatusCategoryInProgress:
		return rmi.RMIStatusInProgress
	case provider.StatusCategoryDone:
		return rmi.RMIStatusCompleted
	case provider.StatusCategoryCanceled:
		return rmi.RMIStatusCancelled
	default:
		return rmi.RMIStatusPlanned
	}
}

func convertOwner(p *provider.Person) string {
	if p == nil {
		return ""
	}
	if p.Name != "" {
		return p.Name
	}
	return p.Email
}

// convertNotes preserves provenance in the item's Notes: the source
// system's own reference and URL, which have no first-class field on
// rmi.RoadmapItem.
func convertNotes(item *provider.Item) string {
	switch {
	case item.SourceRef != "" && item.SourceURL != "":
		return fmt.Sprintf("Source: %s (%s)", item.SourceRef, item.SourceURL)
	case item.SourceURL != "":
		return "Source: " + item.SourceURL
	case item.SourceRef != "":
		return "Source: " + item.SourceRef
	default:
		return ""
	}
}

func formatDate(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func joinErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	err := errs[0]
	for _, e := range errs[1:] {
		err = fmt.Errorf("%w; %w", err, e)
	}
	return err
}
