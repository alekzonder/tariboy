package compose

import (
	"fmt"

	"github.com/alekzonder/tariboy/internal/workflowfile"
)

// WorkflowSpec is a workflows entry of a version 2 file.
type WorkflowSpec struct {
	SourceSpec `yaml:",inline"`
}

func (f File) workflowProblems() []error {
	var out []error
	for _, name := range sortedKeys(f.Workflows) {
		if !workflowfile.ValidName(name) {
			out = append(out, fmt.Errorf("invalid workflow name %q", name))
			continue
		}
		out = append(out, f.sourceProblems("workflow", name, f.Workflows[name].SourceSpec)...)
	}
	return out
}
