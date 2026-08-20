package agentskills

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	maxSkillNameLength     = 64
	maxDescriptionLength   = 1024
	maxCompatibilityLength = 500
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type frontmatter struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       *string           `yaml:"license,omitempty"`
	Compatibility *string           `yaml:"compatibility,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`
	AllowedTools  *string           `yaml:"allowed-tools,omitempty"`
}

func frontmatterBytes(body []byte) ([]byte, error) {
	if !bytes.HasPrefix(body, []byte("---\n")) && !bytes.HasPrefix(body, []byte("---\r\n")) {
		return nil, errors.New("SKILL.md must start with YAML frontmatter")
	}
	lines := bytes.SplitAfter(body, []byte("\n"))
	if len(lines) < 2 {
		return nil, errors.New("SKILL.md frontmatter is not closed")
	}
	start := len(lines[0])
	offset := start
	for _, line := range lines[1:] {
		trimmed := bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r"))
		if bytes.Equal(trimmed, []byte("---")) {
			return body[start:offset], nil
		}
		offset += len(line)
	}
	return nil, errors.New("SKILL.md frontmatter is not closed")
}

func rejectUnsafeYAML(node *yaml.Node) error {
	if node == nil {
		return errors.New("empty YAML frontmatter")
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return errors.New("YAML anchors and aliases are not allowed in SKILL.md frontmatter")
	}
	for _, child := range node.Content {
		if err := rejectUnsafeYAML(child); err != nil {
			return err
		}
	}
	return nil
}

func validateFrontmatterNode(doc *yaml.Node) error {
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return errors.New("SKILL.md frontmatter must be a YAML mapping")
	}
	root := doc.Content[0]
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return errors.New("SKILL.md frontmatter keys must be strings")
		}
		switch key.Value {
		case "name", "description", "license", "compatibility", "allowed-tools":
			if value.Kind != yaml.ScalarNode || value.Tag != "!!str" {
				return fmt.Errorf("frontmatter field %q must be a string", key.Value)
			}
		case "metadata":
			if value.Kind != yaml.MappingNode {
				return errors.New("frontmatter field \"metadata\" must be a string map")
			}
			for j := 0; j < len(value.Content); j += 2 {
				if value.Content[j].Kind != yaml.ScalarNode || value.Content[j].Tag != "!!str" || value.Content[j+1].Kind != yaml.ScalarNode || value.Content[j+1].Tag != "!!str" {
					return errors.New("frontmatter field \"metadata\" must be a string map")
				}
			}
		default:
			return fmt.Errorf("unexpected SKILL.md frontmatter field %q", key.Value)
		}
	}
	return nil
}

func parseFrontmatter(body []byte) (frontmatter, error) {
	raw, err := frontmatterBytes(body)
	if err != nil {
		return frontmatter{}, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return frontmatter{}, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	if err := rejectUnsafeYAML(&doc); err != nil {
		return frontmatter{}, err
	}
	if err := validateFrontmatterNode(&doc); err != nil {
		return frontmatter{}, err
	}
	var trailing yaml.Node
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("multiple YAML documents are not allowed")
		}
		return frontmatter{}, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	var out frontmatter
	if err := doc.Decode(&out); err != nil {
		return frontmatter{}, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	out.Name = strings.TrimSpace(out.Name)
	out.Description = strings.TrimSpace(out.Description)
	if out.Name == "" || !skillNamePattern.MatchString(out.Name) || utf8.RuneCountInString(out.Name) > maxSkillNameLength {
		return frontmatter{}, fmt.Errorf("invalid Agent Skill name %q", out.Name)
	}
	if out.Description == "" || utf8.RuneCountInString(out.Description) > maxDescriptionLength {
		return frontmatter{}, errors.New("Agent Skill description must contain 1 to 1024 characters")
	}
	if out.Compatibility != nil {
		compatibility := strings.TrimSpace(*out.Compatibility)
		if compatibility == "" || utf8.RuneCountInString(compatibility) > maxCompatibilityLength {
			return frontmatter{}, errors.New("Agent Skill compatibility must contain 1 to 500 characters")
		}
		out.Compatibility = &compatibility
	}
	return out, nil
}
