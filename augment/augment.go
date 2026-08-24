// Package augment defines locally-authored data layered on top of synced
// canonical items: prioritization (MoSCoW, Kano, RICE), OKR links, and
// notes. Augments are keyed by the source system's stable reference (e.g.
// an Aha reference number like "MYPROJ-123"), not by the synced row —
// sync overwrites provider data wholesale but never touches augments, so
// local judgments survive every re-sync. Apply overlays augments onto
// items at read time, with augment values winning over provider- or
// fieldmap-derived ones.
package augment

import (
	"time"

	"github.com/grokify/omniroadmap-core/provider"
)

// Metadata keys under which non-canonical augment fields surface on an
// applied Item's Metadata map.
const (
	MetadataKeyOKRRefs = "augment.okr_refs"
	MetadataKeyNotes   = "augment.notes"
)

// ItemAugment is locally-authored data for one item, keyed by
// (Provider, SourceRef).
type ItemAugment struct {
	Provider string `json:"provider"`
	// SourceRef is the source system's human-facing reference (e.g.
	// "MYPROJ-123"); for providers without refs, the source ID.
	SourceRef string `json:"source_ref"`

	// MoSCoW: must_have|should_have|could_have|wont_have; "" = unset.
	MoSCoW string `json:"moscow,omitempty"`
	// Kano: must-be|performance|attractive|indifferent|reverse|
	// questionable; "" = unset.
	Kano string `json:"kano,omitempty"`
	// RICE components; set components override same-name components from
	// the synced item, unset ones leave them intact.
	RICE *provider.RICE `json:"rice,omitempty"`

	// OKRRefs links the item to OKR documents/objectives (e.g.
	// prism-roadmap objective IDs).
	OKRRefs []string `json:"okr_refs,omitempty"`
	Notes   string   `json:"notes,omitempty"`
	// Metadata holds further augmentation fields; on Apply, keys surface on
	// the item's Metadata prefixed "augment.".
	Metadata map[string]any `json:"metadata,omitempty"`

	UpdatedAt time.Time `json:"updated_at"`
}

// Key returns the canonical augment key, "<provider>:<source_ref>".
func (a *ItemAugment) Key() string {
	return a.Provider + ":" + a.SourceRef
}

// IsZero reports whether the augment carries no data (every augmentable
// field unset) — useful for treating a fully-cleared augment as deletable.
func (a *ItemAugment) IsZero() bool {
	return a.MoSCoW == "" && a.Kano == "" && a.RICE == nil &&
		len(a.OKRRefs) == 0 && a.Notes == "" && len(a.Metadata) == 0
}

// Apply overlays augments onto items in place. Matching is by
// (Provider, SourceRef), falling back to (Provider, SourceID) for items
// without a ref. Augment values win over what sync stored.
func Apply(items []provider.Item, augs []ItemAugment) {
	if len(augs) == 0 {
		return
	}
	byKey := make(map[string]*ItemAugment, len(augs))
	for i := range augs {
		byKey[augs[i].Key()] = &augs[i]
	}
	for i := range items {
		item := &items[i]
		aug, ok := byKey[item.Provider+":"+item.SourceRef]
		if !ok {
			aug, ok = byKey[item.Provider+":"+item.SourceID]
		}
		if !ok {
			continue
		}
		applyOne(item, aug)
	}
}

func applyOne(item *provider.Item, aug *ItemAugment) {
	if aug.MoSCoW != "" {
		item.MoSCoW = aug.MoSCoW
	}
	if aug.Kano != "" {
		item.Kano = aug.Kano
	}
	if aug.RICE != nil {
		item.RICE = mergeRICE(item.RICE, aug.RICE)
	}
	if len(aug.OKRRefs) > 0 || aug.Notes != "" || len(aug.Metadata) > 0 {
		if item.Metadata == nil {
			item.Metadata = map[string]any{}
		}
		if len(aug.OKRRefs) > 0 {
			item.Metadata[MetadataKeyOKRRefs] = append([]string(nil), aug.OKRRefs...)
		}
		if aug.Notes != "" {
			item.Metadata[MetadataKeyNotes] = aug.Notes
		}
		for k, v := range aug.Metadata {
			item.Metadata["augment."+k] = v
		}
	}
}

// mergeRICE overlays set components of over onto base, returning a new
// value (neither input is mutated).
func mergeRICE(base, over *provider.RICE) *provider.RICE {
	merged := provider.RICE{}
	if base != nil {
		merged = *base
	}
	if over.Reach != nil {
		merged.Reach = over.Reach
	}
	if over.Impact != nil {
		merged.Impact = over.Impact
	}
	if over.Confidence != nil {
		merged.Confidence = over.Confidence
	}
	if over.Effort != nil {
		merged.Effort = over.Effort
	}
	if over.Score != nil {
		merged.Score = over.Score
	}
	return &merged
}
