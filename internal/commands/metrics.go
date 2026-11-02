package commands

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/metrics"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/store"
)

// The metrics commands read tasks, workflow visits, AI requests, iterations
// and assessments and compute the metrics on request; see
// internal/metrics. Like usage, they serve the operator only.

func metricsCommands() []registry.Command {
	return []registry.Command{
		registry.WithResult[metrics.TaskMetrics](registry.Command{
			Path:    "metrics.task",
			Summary: "Metrics of one task: lead time, status time, usage, rework, questions, quality, agents and versions",
			Args:    []registry.Arg{{Name: "key", Type: registry.String, Required: true, Help: "task key, current or old"}},
			HTTP:    &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/metrics/tasks/{key}"},
			Local:   metricsTaskLocal,
			Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
				return metrics.New(c.Store.Reader(), time.Now).Task(p.Ctx, p.Trimmed("key"))
			},
		}),
		registry.WithResult[metrics.MetricsReport](registry.Command{
			Path:    "metrics.server",
			Summary: "Server metrics over a period, in total and by agent, image version and workflow version",
			Args: []registry.Arg{
				{Name: "agent", Flag: "agent", Short: "a", Type: registry.String, Help: "only this agent's iterations and tasks"},
				{Name: "image", Flag: "image", Short: "i", Type: registry.String, Help: "only iterations and tasks on image NAME[:VERSION]"},
				{Name: "workflow", Flag: "workflow", Short: "w", Type: registry.String, Help: "only iterations and tasks under workflow NAME[:VERSION]; none for no workflow"},
				{Name: "since", Flag: "since", Short: "s", Type: registry.String, Help: "RFC 3339 time (inclusive lower bound; default 30 days before until)"},
				{Name: "until", Flag: "until", Short: "u", Type: registry.String, Help: "RFC 3339 time (exclusive upper bound; default now)"},
			},
			HTTP:  &registry.HTTPRoute{Method: http.MethodGet, Path: "/api/metrics"},
			Local: metricsServerLocal,
			Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
				f, err := metricsFilter(p.Params)
				if err != nil {
					return nil, err
				}
				return metrics.New(c.Store.Reader(), time.Now).Report(p.Ctx, f)
			},
		}),
	}
}

func metricsFilter(p registry.Params) (metrics.Filter, error) {
	var f metrics.Filter
	since, until, err := usageWindow(p)
	if err != nil {
		return f, err
	}
	if since != "" {
		f.Since, _ = store.ParseTime(since)
	}
	if until != "" {
		f.Until, _ = store.ParseTime(until)
	}
	f.Agent = p.Trimmed("agent")
	if f.Image, f.ImageVersion, err = metrics.ParseNameVersion(p.Trimmed("image")); err != nil {
		return f, err
	}
	f.Workflow, f.WorkflowVersion, err = metrics.ParseNameVersion(p.Trimmed("workflow"))
	return f, err
}

func metricsTaskLocal(c *registry.Ctx, p registry.Request) (any, error) {
	m, err := operationCall[metrics.TaskMetrics](client.New(c.Socket), http.MethodGet, "/api/metrics/tasks/"+url.PathEscape(p.Trimmed("key")), nil)
	if err != nil {
		return nil, err
	}
	if c.JSON {
		return nil, printJSON(c.Out, m)
	}
	return nil, printTaskMetrics(c.Out, m)
}

func metricsServerLocal(c *registry.Ctx, p registry.Request) (any, error) {
	query := map[string]string{}
	for _, name := range []string{"agent", "image", "workflow", "since", "until"} {
		if v := p.Trimmed(name); v != "" {
			query[name] = v
		}
	}
	r, err := operationCall[metrics.MetricsReport](client.New(c.Socket), http.MethodGet, "/api/metrics", query)
	if err != nil {
		return nil, err
	}
	if c.JSON {
		return nil, printJSON(c.Out, r)
	}
	return nil, printReport(c.Out, r)
}

// formatDuration prints whole seconds as hours and minutes, "-" for null.
func formatDuration(s metrics.MetricsNullSeconds) string {
	if !s.Valid {
		return "-"
	}
	return fmt.Sprintf("%dh %02dm", s.Value/3600, s.Value%3600/60)
}

func seconds(v int64) string {
	return formatDuration(metrics.MetricsNullSeconds{Value: v, Valid: true})
}

func axisCell(a metrics.MetricsAxisScore) string {
	if a.Count == 0 {
		return "-"
	}
	return strconv.FormatFloat(a.Avg, 'f', -1, 64)
}

// qualityCell prints the average scores image_process/role/task_fit, "-"
// without scores.
func qualityCell(q metrics.MetricsQuality) string {
	if q.ImageProcess.Count+q.Role.Count+q.TaskFit.Count == 0 {
		return "-"
	}
	return axisCell(q.ImageProcess) + "/" + axisCell(q.Role) + "/" + axisCell(q.TaskFit)
}

func tokensTotal(t metrics.MetricsTokens) int64 {
	return t.Input + t.Output + t.CacheWrite + t.CacheRead
}

func printTaskMetrics(out io.Writer, m metrics.TaskMetrics) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	workflows := make([]string, 0, len(m.Workflows))
	for _, v := range m.Workflows {
		workflows = append(workflows, v.Name+":"+v.Version)
	}
	if len(workflows) == 0 {
		workflows = append(workflows, "none")
	}
	u := m.Usage
	fmt.Fprintf(w, "task\t%s %s\n", m.Key, m.Title)
	fmt.Fprintf(w, "status\t%s\n", m.Status)
	fmt.Fprintf(w, "workflow\t%s\n", strings.Join(workflows, ", "))
	fmt.Fprintf(w, "lead time\t%s\n", formatDuration(m.LeadTimeS))
	fmt.Fprintf(w, "iterations\t%d\n", u.Iterations)
	fmt.Fprintf(w, "tokens\tinput %d, output %d, cache write %d, cache read %d\n", u.Tokens.Input, u.Tokens.Output, u.Tokens.CacheWrite, u.Tokens.CacheRead)
	fmt.Fprintf(w, "cost usd\t%.6g\n", u.CostUSD)
	r := m.ReworkByOutcome
	fmt.Fprintf(w, "rework rounds\t%d (changes_requested %d, rework %d, conflict %d)\n", m.ReworkRounds, r.ChangesRequested, r.Rework, r.Conflict)
	fmt.Fprintf(w, "rejected advances\t%d\n", m.RejectedAdvances)
	fmt.Fprintf(w, "customer questions\t%d\n", m.CustomerQuestions)
	fmt.Fprintf(w, "quality\t%s\t(image_process/role/task_fit)\n", qualityCell(m.Quality))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "STATUS\tTIME")
	for _, status := range sortedKeys(m.StatusTimeS) {
		fmt.Fprintf(w, "%s\t%s\n", status, seconds(m.StatusTimeS[status]))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "AGENT\tIMAGE\tVERSION\tDIGEST\tITERATIONS\tTOKENS\tCOST_USD")
	for _, a := range m.Agents {
		for _, img := range a.Images {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%.6g\n", a.Agent, img.ImageRef, img.ImageVersion, shortDigest(img.ImageDigest),
				img.Iterations, tokensTotal(img.Tokens), img.CostUSD)
		}
	}
	return w.Flush()
}

func shortDigest(d string) string {
	if len(d) > 12 {
		return d[:12]
	}
	return d
}

func sortedKeys(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func printReport(out io.Writer, r metrics.MetricsReport) error {
	fmt.Fprintf(out, "period %s .. %s\n", r.Since, r.Until)
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SLICE\tNAME\tVERSION\tITERATIONS\tTOKENS\tCOST_USD\tDONE\tMIXED\tLEAD_MEDIAN\tLEAD_AVG\tREWORK\tREJECTED\tQUESTIONS\tQUALITY")
	row := func(kind string, s metrics.MetricsSlice) {
		t := s.Tasks
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%.6g\t%d\t%d\t%s\t%s\t%d\t%d\t%d\t%s\n", kind, cellOr(s.Name), cellOr(s.Version),
			s.Usage.Iterations, tokensTotal(s.Usage.Tokens), s.Usage.CostUSD, t.Done, t.Mixed,
			formatDuration(t.LeadTimeMedianS), formatDuration(t.LeadTimeAvgS), t.Sum.ReworkRounds, t.Sum.RejectedAdvances,
			t.Sum.CustomerQuestions, qualityCell(t.Quality))
	}
	row("total", r.Total)
	for _, s := range r.Agents {
		row("agent", s)
	}
	for _, s := range r.Images {
		row("image", s)
	}
	for _, s := range r.Workflows {
		row("workflow", s)
	}
	return w.Flush()
}

func cellOr(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
