package commands

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCommandsUseNoRawParamAssertions keeps argument coercion in one place:
// handlers read arguments through the registry.Params accessors after
// registry.Bind, never by asserting a raw map value or through a local helper.
func TestCommandsUseNoRawParamAssertions(t *testing.T) {
	t.Parallel()
	rawAssertion := regexp.MustCompile(`\b(p|params)(\.Params)?\["[^"]+"\]\.\(`)
	removedHelper := regexp.MustCompile(`\b(str|toBool|parseAgentIntParam|agentIntParam|intOf|stringParam|csvParam|rawStringParam|optionalStringParam|optionalPriorityParam|stringSliceParam|int64Param|boolParam|decodeTaskParam|stringSlice)\(`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if rawAssertion.MatchString(line) || removedHelper.MatchString(line) {
				t.Errorf("%s:%d reads a parameter without the registry accessors: %s", file, i+1, strings.TrimSpace(line))
			}
		}
	}
}
