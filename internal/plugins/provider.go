package plugins

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

// provider.go implements the plugin provider contract's subscribe-time gate
// (spec §6.1): a plugin declares the channels it fulfils for parameterized
// subscriptions (Channels.Provide), each with an optional JSON Schema for the
// params a subscriber must supply. When an agent subscribes with params to a
// provided channel, the daemon validates those params here and fails the
// subscribe with the schema error — instead of silently producing nothing.
//
// The validator is a deliberately small JSON Schema subset covering exactly what
// the spec's manifests use: object/array/string/number/integer/boolean/null
// `type`, `required`, `properties`, `items`, `enum`, and boolean
// `additionalProperties`. It is not a general-purpose validator; unknown
// keywords are ignored (accepted), so a schema stays forward-compatible.

// ProviderFor returns the provided-channel declaration and the plugin that owns
// it for an exact channel name, scanning the installed plugin records. ok is
// false when no installed plugin provides that channel (the channel is not a
// provider channel — its params, if any, are opaque and unvalidated).
func ProviderFor(records []Record, channel string) (owner string, p Provided, ok bool) {
	for _, r := range records {
		for _, pr := range r.Channels.Provide {
			if pr.Channel == channel {
				return r.Name, pr, true
			}
		}
	}
	return "", Provided{}, false
}

// IsProviderChannel reports whether channel is declared as a provided channel by
// any installed plugin. Used to accept provider-declared channel names on the
// subscribe path even though they carry arbitrary, plugin-owned prefixes that
// bus.ValidChannel's static prefix list does not know about.
func IsProviderChannel(records []Record, channel string) bool {
	_, _, ok := ProviderFor(records, channel)
	return ok
}

// ValidateSubscribeParams is the subscribe-time gate the daemon wires into the
// bus (bus.SetParamsValidator). It accepts:
//   - a channel no installed plugin provides (not our concern — params opaque);
//   - a provided channel whose declaration has no params_schema (accept any params);
//   - a provided channel whose params satisfy its params_schema.
//
// It rejects params that violate a provided channel's params_schema, returning
// the schema error so the subscribe fails loudly.
func ValidateSubscribeParams(records []Record, channel string, params map[string]any) error {
	_, p, ok := ProviderFor(records, channel)
	if !ok || len(p.ParamsSchema) == 0 {
		return nil
	}
	if err := validateAgainstSchema(p.ParamsSchema, params, ""); err != nil {
		return fmt.Errorf("params for channel %q: %w", channel, err)
	}
	return nil
}

// RecordLister reads the installed plugin records. It matches *Store's List
// signature so the daemon passes its plugin store directly.
type RecordLister interface {
	List() ([]Record, error)
}

// ParamsValidatorFor builds the bus params validator (bus.SetParamsValidator)
// over a record lister. The params gate exists only for provider channels; a
// core channel that no plugin provides never needed the plugin store at all. So
// a transient List() failure must NOT fail every subscribe (spec §6.1, review
// finding #2): on a list error the validator treats the target channel as
// non-provider and accepts the subscribe, reporting the read failure via
// onListErr. When the store IS readable, a provided channel's params_schema gate
// applies exactly as before, failing loudly on bad params.
func ParamsValidatorFor(lister RecordLister, onListErr func(channel string, err error)) func(channel string, params map[string]any) error {
	return func(channel string, params map[string]any) error {
		recs, err := lister.List()
		if err != nil {
			if onListErr != nil {
				onListErr(channel, err)
			}
			return nil // store unreadable: treat as non-provider, allow the subscribe
		}
		return ValidateSubscribeParams(recs, channel, params)
	}
}

// ProvidedChannelInfo is a flattened, presentation-ready view of one plugin
// provided-channel declaration, used by the Messages skill (spec §6.1) to list and
// annotate provider channels — even before the channel row exists in the bus.
type ProvidedChannelInfo struct {
	Channel  string   // the provided channel name
	Provider string   // the plugin that provides it
	Params   []string // param keys drawn from the schema (required ∪ properties)
	Help     string   // the declaration's first help line
}

// ProvidedChannels flattens every installed plugin's Channels.Provide list into
// ProvidedChannelInfo rows, sorted by channel name. Param keys are pulled from
// each declaration's params_schema (its `required` and `properties` keys, unioned
// and sorted); Help is trimmed to its first line. A plugin with no provided
// channels contributes nothing.
func ProvidedChannels(records []Record) []ProvidedChannelInfo {
	var out []ProvidedChannelInfo
	for _, r := range records {
		for _, pr := range r.Channels.Provide {
			out = append(out, ProvidedChannelInfo{
				Channel:  pr.Channel,
				Provider: r.Name,
				Params:   schemaParamKeys(pr.ParamsSchema),
				Help:     firstLine(pr.Help),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Channel < out[j].Channel })
	return out
}

// schemaParamKeys extracts the declared param names from a params_schema: the
// union of its `required` list and its `properties` keys, sorted. A missing or
// unparseable schema yields no keys (nil).
func schemaParamKeys(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var doc schemaDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, k := range doc.Required {
		seen[k] = struct{}{}
	}
	for k := range doc.Properties {
		seen[k] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// firstLine returns the first non-empty line of s, trimmed. Provider help can be
// a multi-line paragraph; the sources command shows only the lead line.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if head, _, found := strings.Cut(s, "\n"); found {
		return strings.TrimSpace(head)
	}
	return s
}

// schemaDoc is the subset of JSON Schema this validator understands. Absent
// keywords impose no constraint. `additionalProperties` accepts a bool (a nested
// schema form is ignored, i.e. treated as permissive) via json.RawMessage.
type schemaDoc struct {
	Type                 string                     `json:"type"`
	Required             []string                   `json:"required"`
	Properties           map[string]json.RawMessage `json:"properties"`
	Items                json.RawMessage            `json:"items"`
	Enum                 []any                      `json:"enum"`
	AdditionalProperties *json.RawMessage           `json:"additionalProperties"`
}

// validateSchemaDocument checks that raw is a structurally sound schema in the
// supported subset: valid JSON, a known `type` (if present), and every nested
// schema (properties/items) likewise sound. It is used at install time so a
// malformed params_schema is caught before the plugin is accepted, not at the
// first subscribe.
func validateSchemaDocument(raw json.RawMessage) error {
	var s schemaDoc
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("not a JSON object: %w", err)
	}
	if s.Type != "" && !knownSchemaType(s.Type) {
		return fmt.Errorf("unknown schema type %q", s.Type)
	}
	for name, ps := range s.Properties {
		if err := validateSchemaDocument(ps); err != nil {
			return fmt.Errorf("property %q: %w", name, err)
		}
	}
	if len(s.Items) > 0 {
		if err := validateSchemaDocument(s.Items); err != nil {
			return fmt.Errorf("items: %w", err)
		}
	}
	return nil
}

func knownSchemaType(t string) bool {
	switch t {
	case "object", "array", "string", "number", "integer", "boolean", "null":
		return true
	}
	return false
}

// validateAgainstSchema validates a decoded JSON value against a schema in the
// supported subset. path is the dotted location used in error messages ("" at
// the root). Numbers are float64 (encoding/json's default) — integer accepts an
// integral float64.
func validateAgainstSchema(raw json.RawMessage, value any, path string) error {
	var s schemaDoc
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("invalid schema at %s: %w", loc(path), err)
	}

	if s.Type != "" && !typeMatches(s.Type, value) {
		return fmt.Errorf("%s: expected %s, got %s", loc(path), s.Type, jsonTypeOf(value))
	}

	if len(s.Enum) > 0 && !enumContains(s.Enum, value) {
		return fmt.Errorf("%s: %v is not one of the allowed values", loc(path), value)
	}

	switch v := value.(type) {
	case map[string]any:
		for _, req := range s.Required {
			if _, present := v[req]; !present {
				return fmt.Errorf("%s: missing required property %q", loc(path), req)
			}
		}
		if s.AdditionalProperties != nil && isFalse(*s.AdditionalProperties) {
			for k := range v {
				if _, declared := s.Properties[k]; !declared {
					return fmt.Errorf("%s: unexpected property %q", loc(path), k)
				}
			}
		}
		// Validate declared properties deterministically (sorted) so error
		// messages are stable across runs.
		names := make([]string, 0, len(s.Properties))
		for name := range s.Properties {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if pv, present := v[name]; present {
				if err := validateAgainstSchema(s.Properties[name], pv, join(path, name)); err != nil {
					return err
				}
			}
		}
	case []any:
		if len(s.Items) > 0 {
			for i, item := range v {
				if err := validateAgainstSchema(s.Items, item, indexPath(path, i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func typeMatches(t string, value any) bool {
	switch t {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "null":
		return value == nil
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		f, ok := value.(float64)
		return ok && f == math.Trunc(f)
	}
	return false
}

func jsonTypeOf(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case nil:
		return "null"
	}
	return "unknown"
}

// enumContains reports whether value deep-equals one of the enum entries. Both
// sides are decoded JSON values (string, float64, bool, nil, []any,
// map[string]any), so reflect.DeepEqual is the right comparison.
func enumContains(enum []any, value any) bool {
	for _, e := range enum {
		if reflect.DeepEqual(e, value) {
			return true
		}
	}
	return false
}

// isFalse reports whether a raw JSON value is the literal boolean false — used
// for `additionalProperties: false`. Any other form (true, a nested schema
// object) is treated as permissive.
func isFalse(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "false"
}

func loc(path string) string {
	if path == "" {
		return "(root)"
	}
	return path
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func indexPath(path string, i int) string {
	return fmt.Sprintf("%s[%d]", path, i)
}
