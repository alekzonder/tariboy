package registry

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/invopop/jsonschema"
)

const (
	jsonSchemaDefsRef = "#/$defs/"
	openAPIRef        = "#/components/schemas/"
)

// WithResult declares the named struct T as c's result type: it derives the
// OpenAPI component schemas of T and every named type T reaches, points
// ResultSchema at T's component, and adds the components to Schemas. Field
// rules follow encoding/json: the json tag names a property, a field without
// omitempty is required, and a struct allows no additional properties. A
// pointer field must carry omitempty. An enum is a string type whose
// JSONSchemaExtend method sets Enum. An invalid type panics at registration.
func WithResult[T any](c Command) Command {
	name, schemas, err := resultSchemas(reflect.TypeFor[T]())
	if err != nil {
		panic(fmt.Sprintf("%s result: %v", c.Path, err))
	}
	if c.Schemas == nil {
		c.Schemas = map[string]map[string]any{}
	}
	for component, schema := range schemas {
		if existing, ok := c.Schemas[component]; ok && !reflect.DeepEqual(existing, schema) {
			panic(fmt.Sprintf("%s result: component schema %s already declared differently", c.Path, component))
		}
		c.Schemas[component] = schema
	}
	c.ResultSchema = map[string]any{"$ref": openAPIRef + name}
	return c
}

func resultSchemas(t reflect.Type) (string, map[string]map[string]any, error) {
	if t.Kind() != reflect.Struct || t.Name() == "" {
		return "", nil, fmt.Errorf("%s is not a named struct", t)
	}
	if err := checkPointerFields(t, map[reflect.Type]bool{}); err != nil {
		return "", nil, err
	}
	data, err := json.Marshal((&jsonschema.Reflector{}).ReflectFromType(t))
	if err != nil {
		return "", nil, err
	}
	// Rewrite JSON Schema references to OpenAPI component references before
	// decoding; a property literally containing the prefix is not expected.
	data = []byte(strings.ReplaceAll(string(data), `"`+jsonSchemaDefsRef, `"`+openAPIRef))
	var root struct {
		Ref  string                    `json:"$ref"`
		Defs map[string]map[string]any `json:"$defs"`
	}
	if err := json.Unmarshal(data, &root); err != nil {
		return "", nil, err
	}
	name := strings.TrimPrefix(root.Ref, openAPIRef)
	if _, ok := root.Defs[name]; !ok {
		return "", nil, fmt.Errorf("%s produced no component schema", t)
	}
	return name, root.Defs, nil
}

// checkPointerFields enforces the response rule that a pointer field is
// optional: the reflector would otherwise mark a nil-able field required.
func checkPointerFields(t reflect.Type, seen map[reflect.Type]bool) error {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array || t.Kind() == reflect.Map {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return nil
	}
	seen[t] = true
	for f := range t.Fields() {
		tag := strings.Split(f.Tag.Get("json"), ",")
		if tag[0] == "-" || (!f.IsExported() && !f.Anonymous) {
			continue
		}
		embedded := f.Anonymous && tag[0] == ""
		if f.Type.Kind() == reflect.Pointer && !embedded && !slices.Contains(tag[1:], "omitempty") {
			return fmt.Errorf("%s.%s: pointer field needs omitempty", t.Name(), f.Name)
		}
		if err := checkPointerFields(f.Type, seen); err != nil {
			return err
		}
	}
	return nil
}
