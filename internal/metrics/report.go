package metrics

import (
	"cmp"
	"maps"
	"math"
	"slices"
	"time"
)

// taskEval is a done task with its metrics and the agents, image versions
// and workflow versions it ran on.
type taskEval struct {
	m         TaskMetrics
	scores    map[string][]int64
	agents    map[string]bool
	images    map[string]imageVer
	workflows map[string]wfVer
}

func (d *data) evaluate(t *taskRec, now time.Time) *taskEval {
	e := &taskEval{m: d.taskMetrics(t, now), scores: t.scores, agents: map[string]bool{},
		images: map[string]imageVer{}, workflows: map[string]wfVer{}}
	for _, it := range t.iters {
		e.agents[it.agent] = true
		e.images[it.image.key()] = it.image
	}
	wfs := d.taskWorkflows(t)
	if len(wfs) == 0 {
		wfs = []wfVer{noWorkflow}
	}
	for _, v := range wfs {
		e.workflows[v.key()] = v
	}
	return e
}

func (f Filter) matchImage(v imageVer) bool {
	return f.Image == "" || (v.name == f.Image && (f.ImageVersion == "" || v.version == f.ImageVersion))
}

func (f Filter) matchWorkflow(v wfVer) bool {
	return f.Workflow == "" || (v.name == f.Workflow && (f.WorkflowVersion == "" || v.version == f.WorkflowVersion))
}

// matchTask keeps a task with an iteration of the agent, an iteration on the
// image and a visit under the workflow the filter names.
func (f Filter) matchTask(e *taskEval) bool {
	if f.Agent != "" && !e.agents[f.Agent] {
		return false
	}
	if f.Image != "" && !slices.ContainsFunc(slices.Collect(maps.Values(e.images)), f.matchImage) {
		return false
	}
	return f.Workflow == "" || slices.ContainsFunc(slices.Collect(maps.Values(e.workflows)), f.matchWorkflow)
}

type sliceAcc struct {
	name, version string
	usage, noTask usageAcc
	tasks         []*taskEval
	mixed         int
}

type sliceSet map[string]*sliceAcc

func (s sliceSet) get(name, version string) *sliceAcc {
	k := name + "\x00" + version
	if s[k] == nil {
		s[k] = &sliceAcc{name: name, version: version}
	}
	return s[k]
}

// place counts a task in the slice of its only version, or as mixed in
// the slice of each of its versions. Versions outside the filter get no slice.
func place[V interface{ key() string }](s sliceSet, e *taskEval, versions map[string]V, nameVersion func(V) (string, string), match func(V) bool) {
	for _, v := range versions {
		if !match(v) {
			continue
		}
		acc := s.get(nameVersion(v))
		if len(versions) == 1 {
			acc.tasks = append(acc.tasks, e)
		} else {
			acc.mixed++
		}
	}
}

func (s sliceSet) list() []MetricsSlice {
	out := make([]MetricsSlice, 0, len(s))
	for _, acc := range s {
		out = append(out, acc.slice())
	}
	slices.SortFunc(out, func(a, b MetricsSlice) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.Version, b.Version))
	})
	return out
}

func (d *data) report(f Filter, rows []iterRow, since, until string, now time.Time) MetricsReport {
	total := &sliceAcc{}
	agents, images, workflows := sliceSet{}, sliceSet{}, sliceSet{}
	for _, it := range rows {
		wf := d.workflowAt(d.task(it.taskKey), it.start)
		if (f.Agent != "" && it.agent != f.Agent) || !f.matchImage(it.image) || !f.matchWorkflow(wf) {
			continue
		}
		for _, acc := range []*sliceAcc{total, agents.get(it.agent, ""), images.get(it.image.name, it.image.version), workflows.get(wf.name, wf.version)} {
			acc.usage.add(it)
			if it.taskKey == "" {
				acc.noTask.add(it)
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(d.tasks)) {
		t := d.tasks[id]
		if t.status != "done" || t.completedAt < since || t.completedAt >= until {
			continue
		}
		e := d.evaluate(t, now)
		if !f.matchTask(e) {
			continue
		}
		total.tasks = append(total.tasks, e)
		for agent := range e.agents {
			if f.Agent == "" || agent == f.Agent {
				acc := agents.get(agent, "")
				acc.tasks = append(acc.tasks, e)
			}
		}
		place(images, e, e.images, func(v imageVer) (string, string) { return v.name, v.version }, f.matchImage)
		place(workflows, e, e.workflows, func(v wfVer) (string, string) { return v.name, v.version }, f.matchWorkflow)
	}
	return MetricsReport{Since: since, Until: until, Total: total.slice(), Agents: agents.list(), Images: images.list(), Workflows: workflows.list()}
}

func (acc *sliceAcc) slice() MetricsSlice {
	return MetricsSlice{Name: acc.name, Version: acc.version, Usage: acc.usage.usage(), UsageWithoutTask: acc.noTask.usage(), Tasks: stats(acc.tasks, acc.mixed)}
}

// stats aggregates the metrics of done tasks.
func stats(tasks []*taskEval, mixed int) MetricsTaskStats {
	s := MetricsTaskStats{Done: len(tasks), Mixed: mixed}
	var leads []int64
	scores := map[string][]int64{}
	for _, e := range tasks {
		m := e.m
		if m.LeadTimeS.Valid {
			leads = append(leads, m.LeadTimeS.Value)
		}
		s.Sum.Iterations += m.Usage.Iterations
		s.Sum.Tokens.add(m.Usage.Tokens)
		s.Sum.CostUSD += m.Usage.CostUSD
		s.Sum.ReworkRounds += m.ReworkRounds
		s.Sum.RejectedAdvances += m.RejectedAdvances
		s.Sum.CustomerQuestions += m.CustomerQuestions
		for axis, v := range e.scores {
			scores[axis] = append(scores[axis], v...)
		}
	}
	s.Sum.CostUSD = roundCost(s.Sum.CostUSD)
	s.Quality = quality(scores)
	if len(leads) > 0 {
		slices.Sort(leads)
		var sum int64
		for _, l := range leads {
			sum += l
		}
		n := len(leads)
		s.LeadTimeAvgS = MetricsNullSeconds{Value: int64(math.Round(float64(sum) / float64(n))), Valid: true}
		s.LeadTimeMedianS = MetricsNullSeconds{Value: (leads[(n-1)/2] + leads[n/2]) / 2, Valid: true}
	}
	if n := float64(len(tasks)); n > 0 {
		avg := func(v int64) float64 { return math.Round(float64(v)/n*100) / 100 }
		s.Avg = &MetricsTaskAverages{Iterations: avg(s.Sum.Iterations), InputTokens: avg(s.Sum.Tokens.Input),
			OutputTokens: avg(s.Sum.Tokens.Output), CacheWriteTokens: avg(s.Sum.Tokens.CacheWrite),
			CacheReadTokens: avg(s.Sum.Tokens.CacheRead), CostUSD: roundCost(s.Sum.CostUSD / n),
			ReworkRounds: avg(s.Sum.ReworkRounds), RejectedAdvances: avg(s.Sum.RejectedAdvances),
			CustomerQuestions: avg(s.Sum.CustomerQuestions)}
	}
	return s
}
