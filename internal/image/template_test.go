package image

import "testing"

func TestValidatePromptTemplateAcceptsGoalRuntime(t *testing.T) {
	template := PromptTemplate{SchemaVersion: 2, Entries: []TemplateEntry{{Kind: "runtime", Runtime: "goal"}}}
	template.SHA256, _ = PromptTemplateHash(template.Entries)
	if err := ValidatePromptTemplate(template); err != nil {
		t.Fatal(err)
	}
}
