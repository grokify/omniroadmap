// Package fieldmap maps tenant-specific custom fields onto canonical
// prioritization fields (MoSCoW, RICE). Some PM-tool tenants — Aha
// workspaces in particular — store MoSCoW and RICE components as custom
// fields; which fields, and with what value vocabulary, varies per tenant.
// A Mapping captures that per-tenant configuration (typically loaded from a
// JSON file) and Apply enriches canonical Items with it after fetch,
// before storage or export.
package fieldmap

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/grokify/omniroadmap-core/provider"
)

// canonical MoSCoW values (matching prism-roadmap's prioritization
// vocabulary; omniroadmap-core's Item.MoSCoW uses the same strings).
const (
	MoSCoWMustHave   = "must_have"
	MoSCoWShouldHave = "should_have"
	MoSCoWCouldHave  = "could_have"
	MoSCoWWontHave   = "wont_have"
)

// canonical Kano Model categories (matching prism-roadmap's KanoCategory
// vocabulary; omniroadmap-core's Item.Kano uses the same strings).
const (
	KanoMustBe       = "must-be"
	KanoPerformance  = "performance"
	KanoAttractive   = "attractive"
	KanoIndifferent  = "indifferent"
	KanoReverse      = "reverse"
	KanoQuestionable = "questionable"
)

// FieldRule selects a custom field by key and optionally normalizes its
// values.
type FieldRule struct {
	// Key is the custom field key to read from Item.CustomFields.
	Key string `json:"key"`

	// Values optionally normalizes raw field values to canonical ones,
	// e.g. {"Must": "must_have", "M": "must_have"}. Matching is
	// case-insensitive on the raw value. When empty, the raw value is used
	// as-is (after lowercasing/underscoring for MoSCoW).
	Values map[string]string `json:"values,omitempty"`
}

// Mapping is a per-tenant configuration mapping custom fields to canonical
// prioritization fields. Nil rules mean "this tenant doesn't store that
// component".
type Mapping struct {
	// Description is optional free-text documentation (e.g. which tenant
	// this mapping belongs to).
	Description string `json:"description,omitempty"`

	MoSCoW *FieldRule `json:"moscow,omitempty"`
	Kano   *FieldRule `json:"kano,omitempty"`

	// RICE component rules. Values read through these are coerced to
	// numbers (raw numeric custom fields, or numeric strings; Values
	// normalization applies first, so e.g. {"High": "2.0"} works).
	Reach      *FieldRule `json:"reach,omitempty"`
	Impact     *FieldRule `json:"impact,omitempty"`
	Confidence *FieldRule `json:"confidence,omitempty"`
	Effort     *FieldRule `json:"effort,omitempty"`
	Score      *FieldRule `json:"score,omitempty"`
}

// Load reads a Mapping from a JSON file.
func Load(path string) (*Mapping, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is caller-supplied config
	if err != nil {
		return nil, fmt.Errorf("fieldmap: reading %s: %w", path, err)
	}
	var m Mapping
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("fieldmap: parsing %s: %w", path, err)
	}
	return &m, nil
}

// Apply enriches items in place, populating Item.MoSCoW and Item.RICE from
// Item.CustomFields per the mapping. Items whose mapped fields are absent
// are left untouched. A nil mapping is a no-op.
func Apply(items []provider.Item, m *Mapping) {
	if m == nil {
		return
	}
	for i := range items {
		applyOne(&items[i], m)
	}
}

func applyOne(item *provider.Item, m *Mapping) {
	if m.MoSCoW != nil {
		if raw, ok := customFieldValue(item.CustomFields, m.MoSCoW.Key); ok {
			if v := normalizeMoSCoW(raw, m.MoSCoW.Values); v != "" {
				item.MoSCoW = v
			}
		}
	}

	if m.Kano != nil {
		if raw, ok := customFieldValue(item.CustomFields, m.Kano.Key); ok {
			if v := normalizeKano(raw, m.Kano.Values); v != "" {
				item.Kano = v
			}
		}
	}

	rice := provider.RICE{}
	populated := false
	assign := func(rule *FieldRule, dst **float64) {
		if rule == nil {
			return
		}
		raw, ok := customFieldValue(item.CustomFields, rule.Key)
		if !ok {
			return
		}
		if f, ok := coerceFloat(raw, rule.Values); ok {
			*dst = &f
			populated = true
		}
	}
	assign(m.Reach, &rice.Reach)
	assign(m.Impact, &rice.Impact)
	assign(m.Confidence, &rice.Confidence)
	assign(m.Effort, &rice.Effort)
	assign(m.Score, &rice.Score)

	if populated {
		item.RICE = &rice
	}
}

// customFieldValue finds a custom field by key and returns its raw value,
// unwrapping raw-JSON byte slices into native Go values.
func customFieldValue(fields []provider.CustomField, key string) (any, bool) {
	for _, f := range fields {
		if f.Key == key {
			return unwrapRawJSON(f.Value), true
		}
	}
	return nil, false
}

// unwrapRawJSON converts byte-slice values (e.g. aha-go's jx.Raw, which
// holds unparsed JSON) into native Go values. Non-byte-slice values pass
// through unchanged. Uses reflection so named byte-slice types from any
// SDK are handled without importing them.
func unwrapRawJSON(raw any) any {
	rv := reflect.ValueOf(raw)
	if rv.Kind() != reflect.Slice || rv.Type().Elem().Kind() != reflect.Uint8 {
		return raw
	}
	var v any
	if err := json.Unmarshal(rv.Bytes(), &v); err != nil {
		return raw
	}
	return v
}

// normalizeMoSCoW converts a raw custom-field value into a canonical MoSCoW
// string, via the rule's Values table first, then a built-in normalization
// (lowercase, spaces/hyphens→underscores, common aliases). Returns "" when
// the value can't be normalized to a valid MoSCoW level.
func normalizeMoSCoW(raw any, values map[string]string) string {
	s := stringify(raw)
	if s == "" {
		return ""
	}
	if v, ok := lookupFold(values, s); ok {
		s = v
	}
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "_", "-", "_", "'", "").Replace(s)
	switch s {
	case MoSCoWMustHave, "must":
		return MoSCoWMustHave
	case MoSCoWShouldHave, "should":
		return MoSCoWShouldHave
	case MoSCoWCouldHave, "could":
		return MoSCoWCouldHave
	case MoSCoWWontHave, "wont", "wont_have_this_time", "will_not_have":
		return MoSCoWWontHave
	default:
		return ""
	}
}

// normalizeKano converts a raw custom-field value into a canonical Kano
// category, via the rule's Values table first, then a built-in
// normalization (lowercase, spaces/underscores→hyphens, common aliases).
// Returns "" when the value can't be normalized to a valid category.
func normalizeKano(raw any, values map[string]string) string {
	s := stringify(raw)
	if s == "" {
		return ""
	}
	if v, ok := lookupFold(values, s); ok {
		s = v
	}
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "-", "_", "-").Replace(s)
	switch s {
	case KanoMustBe, "basic", "threshold", "expected":
		return KanoMustBe
	case KanoPerformance, "one-dimensional", "linear":
		return KanoPerformance
	case KanoAttractive, "delighter", "excitement", "exciter":
		return KanoAttractive
	case KanoIndifferent:
		return KanoIndifferent
	case KanoReverse:
		return KanoReverse
	case KanoQuestionable:
		return KanoQuestionable
	default:
		return ""
	}
}

// coerceFloat converts a raw custom-field value to a float64, applying the
// rule's Values normalization first (so label vocabularies like
// {"High": "2.0"} work). Handles float64, int, json.Number, and numeric
// strings.
func coerceFloat(raw any, values map[string]string) (float64, bool) {
	if s := stringify(raw); s != "" {
		if v, ok := lookupFold(values, s); ok {
			raw = v
		}
	}
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// stringify renders a raw custom-field value as a string for lookup and
// normalization purposes.
func stringify(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

// lookupFold does a case-insensitive lookup in a normalization table.
func lookupFold(values map[string]string, key string) (string, bool) {
	if len(values) == 0 {
		return "", false
	}
	if v, ok := values[key]; ok {
		return v, true
	}
	for k, v := range values {
		if strings.EqualFold(k, strings.TrimSpace(key)) {
			return v, true
		}
	}
	return "", false
}
