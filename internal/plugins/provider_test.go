package plugins

import (
	"encoding/json"
	"testing"
)

// providerRecords builds the installed-plugin set the subscribe gate scans: one
// provider with a params_schema on issue-provider:query, one with a schema-less
// provided channel on issue-provider:ticket.
func providerRecords() []Record {
	return []Record{{
		Name: "issue-provider",
		Channels: Channels{
			Publish: []string{"issue-provider:*"},
			Provide: []Provided{
				{
					Channel: "issue-provider:query",
					ParamsSchema: json.RawMessage(`{"type":"object","required":["query"],
						"properties":{"query":{"type":"string"},
						              "limit":{"type":"integer"}},
						"additionalProperties":false}`),
				},
				{Channel: "issue-provider:ticket"}, // no schema → accepts any params
			},
		},
	}}
}

func TestValidateSubscribeParams(t *testing.T) {
	recs := providerRecords()
	cases := []struct {
		name    string
		channel string
		params  map[string]any
		wantErr bool
	}{
		{"valid params", "issue-provider:query", map[string]any{"query": "is:open"}, false},
		{"valid with optional int", "issue-provider:query", map[string]any{"query": "x", "limit": float64(5)}, false},
		{"missing required", "issue-provider:query", map[string]any{"limit": float64(5)}, true},
		{"wrong type", "issue-provider:query", map[string]any{"query": float64(1)}, true},
		{"non-integer for integer", "issue-provider:query", map[string]any{"query": "x", "limit": float64(1.5)}, true},
		{"unexpected property", "issue-provider:query", map[string]any{"query": "x", "junk": true}, true},
		{"no-schema provider accepts anything", "issue-provider:ticket", map[string]any{"whatever": []any{1.0, 2.0}}, false},
		{"non-provider channel: params opaque", "chat:room", map[string]any{"anything": "goes"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateSubscribeParams(recs, c.channel, c.params)
			if c.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestProviderForAndIsProviderChannel(t *testing.T) {
	recs := providerRecords()
	if owner, p, ok := ProviderFor(recs, "issue-provider:query"); !ok || owner != "issue-provider" || p.Channel != "issue-provider:query" {
		t.Fatalf("ProviderFor(issue-provider:query) = %q %+v %v", owner, p, ok)
	}
	if _, _, ok := ProviderFor(recs, "issue-provider:unknown"); ok {
		t.Fatalf("ProviderFor(issue-provider:unknown) should be not found")
	}
	if !IsProviderChannel(recs, "issue-provider:ticket") {
		t.Fatalf("issue-provider:ticket should be a provider channel")
	}
	if IsProviderChannel(recs, "chat:room") {
		t.Fatalf("chat:room is not a provider channel")
	}
}

// TestProvidedChannels flattens provider declarations into presentation rows:
// sorted by channel, param keys unioned from required+properties, help trimmed
// to its first line, and schema-less channels contributing no params.
func TestProvidedChannels(t *testing.T) {
	recs := []Record{
		{
			Name: "issue-provider",
			Channels: Channels{
				Publish: []string{"issue-provider:*"},
				Provide: []Provided{
					{
						Channel: "issue-provider:query",
						ParamsSchema: json.RawMessage(`{"type":"object","required":["query"],
							"properties":{"query":{"type":"string"},"limit":{"type":"integer"}}}`),
						Help: "Subscribe with params {query: ...}.\nSecond line ignored.",
					},
					{Channel: "issue-provider:ticket"}, // no schema → no params
				},
			},
		},
	}
	got := ProvidedChannels(recs)
	if len(got) != 2 {
		t.Fatalf("want 2 provided channels, got %d: %+v", len(got), got)
	}
	// Sorted by channel: issue-provider:query before issue-provider:ticket.
	if got[0].Channel != "issue-provider:query" || got[1].Channel != "issue-provider:ticket" {
		t.Fatalf("unexpected order: %q, %q", got[0].Channel, got[1].Channel)
	}
	q := got[0]
	if q.Provider != "issue-provider" {
		t.Fatalf("provider = %q, want issue-provider", q.Provider)
	}
	if len(q.Params) != 2 || q.Params[0] != "limit" || q.Params[1] != "query" {
		t.Fatalf("params = %v, want [limit query]", q.Params)
	}
	if q.Help != "Subscribe with params {query: ...}." {
		t.Fatalf("help = %q, want first line only", q.Help)
	}
	if len(got[1].Params) != 0 {
		t.Fatalf("issue-provider:ticket params = %v, want none", got[1].Params)
	}
	if ProvidedChannels(nil) != nil {
		t.Fatalf("ProvidedChannels(nil) should be nil")
	}
}

// TestValidateAgainstSchemaSubset exercises the schema keywords the validator
// supports directly, including nested items + enum (the issue-provider:ticket example
// in the spec).
func TestValidateAgainstSchemaSubset(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"required":["ticket"],
		"properties":{
			"ticket":{"type":"string"},
			"events":{"type":"array","items":{"enum":["comment","status"]}}
		}
	}`)
	good := []map[string]any{
		{"ticket": "PROJ-42"},
		{"ticket": "PROJ-42", "events": []any{"comment"}},
		{"ticket": "PROJ-42", "events": []any{"comment", "status"}},
	}
	for _, g := range good {
		if err := validateAgainstSchema(schema, g, ""); err != nil {
			t.Errorf("expected %v valid, got %v", g, err)
		}
	}
	bad := []map[string]any{
		{},                                   // missing required ticket
		{"ticket": 1.0},                      // ticket wrong type
		{"ticket": "x", "events": "comment"}, // events not an array
		{"ticket": "x", "events": []any{"bogus"}},        // enum violation
		{"ticket": "x", "events": []any{"comment", 1.0}}, // second item enum violation
	}
	for _, b := range bad {
		if err := validateAgainstSchema(schema, b, ""); err == nil {
			t.Errorf("expected %v invalid, got nil", b)
		}
	}
}
