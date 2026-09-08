package imagefile

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var imageVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+)(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

func validateVersionField(data []byte) error {
	var fields map[string]yaml.Node
	if err := yaml.Unmarshal(data, &fields); err != nil {
		return err
	}
	node, present := fields["image_version"]
	if !present {
		return nil
	}
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return fmt.Errorf("image_version must be a SemVer string")
	}
	return ValidateImageVersion(node.Value)
}

// ValidateImageVersion accepts a complete SemVer 2.0.0 version without a v prefix.
func ValidateImageVersion(value string) error {
	match := imageVersionPattern.FindStringSubmatch(value)
	if match != nil {
		valid := true
		for _, part := range strings.Split(strings.TrimPrefix(match[4], "-"), ".") {
			if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
				valid = false
			}
		}
		if valid {
			return nil
		}
	}
	return fmt.Errorf("image_version must be a SemVer version (e.g. 1.2.3), got %q", value)
}

func readVersion(path string) (string, *yaml.Node, *yaml.Node, error) {
	if path == "" {
		path = "."
	}
	path, err := sourcePath(path)
	if err != nil {
		return "", nil, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return "", nil, nil, err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return "", nil, nil, fmt.Errorf("expected one YAML document")
	}
	// Decode the mapping as well to reject duplicate keys before modifying it.
	var fields map[string]any
	if err := doc.Decode(&fields); err != nil {
		return "", nil, nil, err
	}
	if len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		fields := doc.Content[0].Content
		for i := 0; i < len(fields); i += 2 {
			if fields[i].Value != "image_version" {
				continue
			}
			node := fields[i+1]
			if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
				return "", nil, nil, fmt.Errorf("image_version must be a SemVer string")
			}
			if err := ValidateImageVersion(node.Value); err != nil {
				return "", nil, nil, err
			}
			return path, &doc, node, nil
		}
	}
	return "", nil, nil, fmt.Errorf("image_version is required")
}

func GetVersion(path string) (string, error) {
	_, _, node, err := readVersion(path)
	if err != nil {
		return "", err
	}
	return node.Value, nil
}

func UpdateVersion(path, part string) (string, error) {
	index := -1
	switch part {
	case "major":
		index = 0
	case "minor":
		index = 1
	case "patch":
		index = 2
	}
	if index < 0 {
		return "", fmt.Errorf("version update requires major, minor, or patch")
	}
	path, doc, node, err := readVersion(path)
	if err != nil {
		return "", err
	}
	parts := imageVersionPattern.FindStringSubmatch(node.Value)[1:4]
	value, _ := new(big.Int).SetString(parts[index], 10)
	parts[index] = value.Add(value, big.NewInt(1)).String()
	for i := index + 1; i < len(parts); i++ {
		parts[i] = "0"
	}
	node.Value = strings.Join(parts, ".")
	data, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	// Resolve an explicitly supplied symlink so replacement updates its target.
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".image-version-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), info.Mode().Perm()); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return node.Value, nil
}
