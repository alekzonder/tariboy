package tfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestMarshalRoundTripAndOrder(t *testing.T) {
	in := &File{
		SchemaVersion: 3,
		ImageVersion:  "1.2.3",
		Extends:       []string{"./base"},
		Plugins:       []Plugin{{Name: "p"}},
		Skills: []SkillEntry{
			{Dir: "/abs/skill"},
			{Git: "https://example.com/r.git", Path: "skills/a", Ref: "abc", Only: "tariboy"},
			{URL: "https://example.com/s.tgz"},
		},
		Prompts: []PromptEntry{
			{File: "./p.md"},
			{Git: "https://example.com/r.git", Path: "prompts/p.md", Ref: "abc"},
			{URL: "https://example.com/p.md", Only: "tariboy"},
			{Runtime: "identity"},
		},
	}
	data, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	last := -1
	for _, key := range []string{"schema_version:", "image_version:", "extends:", "plugins:", "skills:", "prompts:"} {
		i := strings.Index(text, "\n"+key)
		if key == "schema_version:" {
			i = strings.Index(text, key)
		}
		if i <= last {
			t.Fatalf("key %s out of order in\n%s", key, text)
		}
		last = i
	}
	if !strings.Contains(text, "- git: https://example.com/r.git\n    path: skills/a\n    ref: abc\n    only: tariboy\n") {
		t.Fatalf("entry field order wrong:\n%s", text)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, Filename), data, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := Parse(dir)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, text)
	}
	out.Dir = ""
	for i := range out.Skills {
		out.Skills[i].BaseDir = ""
	}
	for i := range out.Prompts {
		out.Prompts[i].BaseDir = ""
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip differs:\n%+v\n%+v", in, out)
	}
}

func TestMarshalEmptyListsParse(t *testing.T) {
	data, err := Marshal(&File{SchemaVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, Filename), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(dir); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
}
