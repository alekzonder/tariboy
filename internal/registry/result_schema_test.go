package registry

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/invopop/jsonschema"
)

type testColor string

func (testColor) JSONSchemaExtend(s *jsonschema.Schema) {
	s.Enum = []any{"red", "blue"}
}

type testItem struct {
	ID string `json:"id"`
}

type testResult struct {
	Name     string            `json:"name"`
	Note     string            `json:"note,omitempty"`
	Count    *int              `json:"count,omitempty"`
	Color    testColor         `json:"color"`
	Items    []testItem        `json:"items"`
	Labels   map[string]string `json:"labels"`
	Optional []string          `json:"optional" jsonschema_extras:"nullable=true"`
}

type testBadPointer struct {
	Count *int `json:"count"`
}

type testNestedBadPointer struct {
	Items []testBadPointer `json:"items"`
}

func TestWithResultDerivesComponentSchemas(t *testing.T) {
	c := WithResult[testResult](cmd("a.b", "GET", "/api/a/b"))
	if !reflect.DeepEqual(c.ResultSchema, map[string]any{"$ref": "#/components/schemas/testResult"}) {
		t.Fatalf("ResultSchema = %#v", c.ResultSchema)
	}
	root := c.Schemas["testResult"]
	if root == nil {
		t.Fatalf("component missing: %#v", c.Schemas)
	}
	if root["type"] != "object" || root["additionalProperties"] != false {
		t.Fatalf("root = %#v", root)
	}
	required := map[string]bool{}
	for _, name := range root["required"].([]any) {
		required[name.(string)] = true
	}
	for _, name := range []string{"name", "color", "items", "labels", "optional"} {
		if !required[name] {
			t.Errorf("%s not required: %v", name, root["required"])
		}
	}
	for _, name := range []string{"note", "count"} {
		if required[name] {
			t.Errorf("omitempty field %s required", name)
		}
	}
	props := root["properties"].(map[string]any)
	if !reflect.DeepEqual(props["color"], map[string]any{"type": "string", "enum": []any{"red", "blue"}}) {
		t.Fatalf("color = %#v", props["color"])
	}
	if got := props["items"].(map[string]any)["items"].(map[string]any)["$ref"]; got != "#/components/schemas/testItem" {
		t.Fatalf("items = %#v", props["items"])
	}
	if c.Schemas["testItem"]["additionalProperties"] != false {
		t.Fatalf("nested component = %#v", c.Schemas["testItem"])
	}
	if props["optional"].(map[string]any)["nullable"] != true {
		t.Fatalf("optional = %#v", props["optional"])
	}
	data, err := json.Marshal(c.Schemas)
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"$defs", "$schema", "$id"} {
		if strings.Contains(string(data), banned) {
			t.Fatalf("schemas contain %s: %s", banned, data)
		}
	}
}

func TestWithResultRejectsRequiredPointer(t *testing.T) {
	for name, build := range map[string]func(){
		"top level": func() { WithResult[testBadPointer](cmd("a.b", "GET", "/api/a/b")) },
		"nested":    func() { WithResult[testNestedBadPointer](cmd("a.b", "GET", "/api/a/b")) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil || !strings.Contains(r.(string), "omitempty") {
					t.Fatalf("panic = %v, want pointer omitempty error", r)
				}
			}()
			build()
		})
	}
}

func TestWithResultRejectsConflictingComponent(t *testing.T) {
	c := cmd("a.b", "GET", "/api/a/b")
	c.Schemas = map[string]map[string]any{"testItem": {"type": "string"}}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "testItem") {
			t.Fatalf("panic = %v, want component conflict", r)
		}
	}()
	WithResult[testResult](c)
}
