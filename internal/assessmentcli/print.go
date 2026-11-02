package assessmentcli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"text/tabwriter"

	"github.com/alekzonder/tariboy/internal/assessment"
)

func printResult(cmd string, raw json.RawMessage, out io.Writer) error {
	if cmd == "ls" {
		var list assessment.AssessmentList
		if err := json.Unmarshal(raw, &list); err != nil {
			return err
		}
		return printList(list, out)
	}
	var res assessment.AssessmentShowResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return err
	}
	return printAssessment(res.Assessment, out)
}

// scoreOf is the score of axis name, or - when it is not applicable.
func scoreOf(axes []assessment.AssessmentAxis, name string) string {
	for _, a := range axes {
		if a.Axis == name && a.Score != nil {
			return strconv.Itoa(*a.Score)
		}
	}
	return "-"
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func printList(list assessment.AssessmentList, out io.Writer) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCREATED\tAGENT\tIMAGE\tVERSION\tITERATION\tTASK\tPROCESS\tROLE\tTASK_FIT\tVIOLATIONS")
	for _, a := range list.Assessments {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n", a.ID, a.CreatedAt, dash(a.SubjectAgent),
			dash(a.ImageRef), dash(a.ImageVersion), dash(a.IterationID), dash(a.TaskKey),
			scoreOf(a.Axes, assessment.AxisImageProcess), scoreOf(a.Axes, assessment.AxisRole),
			scoreOf(a.Axes, assessment.AxisTaskFit), a.ViolationCount)
	}
	return w.Flush()
}

func evidence(e assessment.AssessmentEvidence) string {
	switch e.Kind {
	case assessment.EvidenceTranscript:
		return fmt.Sprintf("transcript %s %s", e.IterationID, e.Locator)
	case assessment.EvidenceTaskEvent:
		return fmt.Sprintf("task_event %s#%d", e.TaskKey, e.Sequence)
	}
	return fmt.Sprintf("%s %s", e.Kind, e.RunID)
}

func printAssessment(a assessment.Assessment, out io.Writer) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, row := range [][2]string{
		{"ID", a.ID}, {"Created", a.CreatedAt}, {"Rubric", a.RubricVersion},
		{"Assessor", fmt.Sprintf("%s (%s %s)", a.AssessorAgent, dash(a.AssessorImageRef), dash(a.AssessorImageVersion))},
		{"Iteration", dash(a.IterationID)}, {"Task", dash(a.TaskKey)}, {"Agent", dash(a.SubjectAgent)},
		{"Image", fmt.Sprintf("%s %s %s", dash(a.ImageRef), dash(a.ImageVersion), dash(a.ImageDigest))},
		{"Workflow", fmt.Sprintf("%s %s", dash(a.WorkflowVersion), dash(a.WorkflowDigest))},
		{"Summary", a.Summary},
	} {
		fmt.Fprintf(w, "%s:\t%s\n", row[0], row[1])
	}
	fmt.Fprintln(w, "\nAXIS\tVERDICT\tSCORE\tNOTE")
	for _, x := range a.Axes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", x.Axis, x.Verdict, scoreOf([]assessment.AssessmentAxis{x}, x.Axis), x.Note)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(out, "\nViolations: %d\n", len(a.Violations))
	for i, v := range a.Violations {
		fmt.Fprintf(out, "%d. [%s] %s\n   quote: %q\n   evidence: %s\n", i+1, v.Axis, v.Description, v.Quote, evidence(v.Evidence))
	}
	return nil
}
