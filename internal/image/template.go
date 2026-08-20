package image

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

const maxPromptTemplateEntries = 1024

type TemplateEntry struct {
	Kind        string `json:"kind"`
	Runtime     string `json:"runtime,omitempty"`
	Source      string `json:"source,omitempty"`
	Category    string `json:"category,omitempty"`
	ArchivePath string `json:"archive_path,omitempty"`
	Size        int64  `json:"size,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

type PromptTemplate struct {
	SchemaVersion int             `json:"schema_version"`
	Entries       []TemplateEntry `json:"entries"`
	SHA256        string          `json:"sha256"`
}

func templateHash(entries []TemplateEntry) (string, error) {
	canonical, err := json.Marshal(struct {
		SchemaVersion int             `json:"schema_version"`
		Entries       []TemplateEntry `json:"entries"`
	}{2, entries})
	if err != nil {
		return "", err
	}
	return sha256hex(canonical), nil
}

func PromptTemplateHash(entries []TemplateEntry) (string, error) {
	return templateHash(entries)
}

func ValidatePromptTemplate(template PromptTemplate) error {
	if template.SchemaVersion != 2 {
		return fmt.Errorf("unsupported prompt template schema_version %d", template.SchemaVersion)
	}
	if len(template.Entries) > maxPromptTemplateEntries {
		return fmt.Errorf("prompt template has %d entries, maximum is %d", len(template.Entries), maxPromptTemplateEntries)
	}
	want, err := templateHash(template.Entries)
	if err != nil {
		return err
	}
	if template.SHA256 != want {
		return fmt.Errorf("prompt template digest mismatch")
	}
	seenRuntime := map[string]bool{}
	seenLayers := map[string]bool{}
	validRuntime := map[string]bool{"identity": true, "context": true, "messages": true, "awaiting-replies": true, "user-prompt": true, "one-shot": true, "workdir": true}
	for i, entry := range template.Entries {
		switch entry.Kind {
		case "runtime":
			if !validRuntime[entry.Runtime] || entry.ArchivePath != "" || seenRuntime[entry.Runtime] {
				return fmt.Errorf("invalid runtime template entry %d", i)
			}
			seenRuntime[entry.Runtime] = true
		case "file":
			clean := path.Clean(entry.ArchivePath)
			if entry.Runtime != "" || clean != entry.ArchivePath || !strings.HasPrefix(clean, "prompt/layers/") || seenLayers[clean] || entry.Size < 0 || len(entry.SHA256) != 64 {
				return fmt.Errorf("invalid file template entry %d", i)
			}
			seenLayers[clean] = true
		default:
			return fmt.Errorf("invalid template entry %d kind %q", i, entry.Kind)
		}
	}
	return nil
}

func (s *Store) ReadTemplate(ref Ref) (PromptTemplate, error) {
	body, err := readFileFromTar(s.tarPath(ref), "prompt/template.json")
	if err != nil {
		return PromptTemplate{}, err
	}
	var template PromptTemplate
	if err := json.Unmarshal(body, &template); err != nil {
		return PromptTemplate{}, fmt.Errorf("parse prompt template: %w", err)
	}
	if err := ValidatePromptTemplate(template); err != nil {
		return PromptTemplate{}, err
	}
	return template, nil
}
