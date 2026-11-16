package tfile

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var imageVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+)(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)

// validateVersionField checks image_version, when present, as a SemVer string
// (same rule as internal/imagefile/version.go).
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
	return validateSemVer("image_version", node.Value)
}

func validateSemVer(key, value string) error {
	if match := imageVersionPattern.FindStringSubmatch(value); match != nil {
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
	return fmt.Errorf("%s must be a SemVer version (e.g. 1.2.3), got %q", key, value)
}

// ValidImageVersion checks an image_version value: a complete SemVer 2.0.0
// version without a v prefix (internal/imagefile ValidateImageVersion).
func ValidImageVersion(value string) error {
	return validateSemVer("image_version", value)
}
