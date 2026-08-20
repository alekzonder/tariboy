package agentskills

import (
	"strings"
	"testing"
)

func TestParseFrontmatterAcceptsAgentSkillsFields(t *testing.T) {
	body := []byte(`---
name: code-review
description: Review changes safely when asked for code review.
license: Apache-2.0
compatibility: Requires git.
metadata:
  author: example-org
  version: "1.0"
allowed-tools: Bash(git:*) Read
---
# Instructions
`)
	got, err := parseFrontmatter(body)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "code-review" || got.Description != "Review changes safely when asked for code review." {
		t.Fatalf("frontmatter = %#v", got)
	}
}

func TestParseFrontmatterRejectsInvalidDocuments(t *testing.T) {
	tests := map[string]string{
		"missing opening delimiter": "name: code-review\ndescription: review\n---\n",
		"missing closing delimiter": "---\nname: code-review\ndescription: review\n",
		"malformed yaml":            "---\nname: [\n---\n",
		"not a mapping":             "---\n- name\n- description\n---\n",
		"missing name":              "---\ndescription: review\n---\n",
		"missing description":       "---\nname: code-review\n---\n",
		"unknown field":             "---\nname: code-review\ndescription: review\ncustom: value\n---\n",
		"duplicate field":           "---\nname: code-review\nname: other\ndescription: review\n---\n",
		"yaml alias":                "---\nname: &name code-review\ndescription: *name\n---\n",
		"consecutive hyphens":       "---\nname: code--review\ndescription: review\n---\n",
		"uppercase name":            "---\nname: Code-review\ndescription: review\n---\n",
		"empty description":         "---\nname: code-review\ndescription: ''\n---\n",
		"non-string metadata":       "---\nname: code-review\ndescription: review\nmetadata: {version: 1}\n---\n",
		"long compatibility":        "---\nname: code-review\ndescription: review\ncompatibility: " + strings.Repeat("x", 501) + "\n---\n",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseFrontmatter([]byte(body)); err == nil {
				t.Fatal("accepted invalid frontmatter")
			}
		})
	}
}

func TestParseFrontmatterEnforcesDescriptionLimit(t *testing.T) {
	body := []byte("---\nname: code-review\ndescription: " + strings.Repeat("x", 1025) + "\n---\n")
	if _, err := parseFrontmatter(body); err == nil {
		t.Fatal("accepted description over 1024 characters")
	}
}
