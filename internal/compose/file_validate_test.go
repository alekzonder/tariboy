package compose

import "testing"

// validationStep is one violation of a chain: Validate must report want, and
// fix then removes that violation so the next one becomes the first.
type validationStep struct {
	want string
	fix  func()
}

func runValidationChain(t *testing.T, f *File, steps []validationStep) {
	t.Helper()
	for _, step := range steps {
		err := f.Validate()
		if err == nil {
			t.Fatalf("Validate() = nil, want %q", step.want)
		}
		if err.Error() != step.want {
			t.Fatalf("Validate() = %q, want %q", err, step.want)
		}
		step.fix()
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() after every fix = %v", err)
	}
}

func TestValidateReportsSectionsInOrder(t *testing.T) {
	t.Parallel()
	f := File{
		Version:    2,
		Images:     map[string]ImageSpec{"Bad Image!": {}},
		Groups:     map[string]GroupSpec{"Bad Group!": {}},
		Agents:     map[string]AgentSpec{"Bad Agent!": {}},
		TaskQueues: map[string]TaskQueueSpec{"bad": {}},
	}
	runValidationChain(t, &f, []validationStep{
		{`unsupported compose version 2 (want 1)`, func() { f.Version = 1 }},
		{`invalid image name "Bad Image!"`, func() { f.Images = nil }},
		{`invalid group name "Bad Group!"`, func() { f.Groups = nil }},
		{`invalid agent name "Bad Agent!"`, func() { f.Agents = nil }},
		{`invalid task queue prefix "bad"`, func() { f.TaskQueues = nil }},
	})
}

func TestValidateGroupChecksInOrder(t *testing.T) {
	t.Parallel()
	g := GroupSpec{Lead: "ghost", Budget: &BudgetSpec{LimitUSD: 1, Mode: "wat"}}
	f := File{Version: 1, Groups: map[string]GroupSpec{"team": g}, Agents: map[string]AgentSpec{"other": {Image: "i", Group: "team"}}}
	runValidationChain(t, &f, []validationStep{
		{`group "team" lead "ghost" is not a member of that group`, func() {
			g.Lead = "other"
			f.Groups["team"] = g
		}},
		{`group "team" budget: invalid budget mode "wat" (want warn|block|enforce)`, func() {
			g.Budget = nil
			f.Groups["team"] = g
		}},
	})
}

func TestValidateAgentChecksInOrder(t *testing.T) {
	t.Parallel()
	negative := -1
	a := AgentSpec{
		Group:   "Bad Group!",
		Budget:  &BudgetSpec{LimitUSD: 1, Mode: "wat"},
		Harness: &HarnessSpec{},
		Timeout: "60",
		Goal:    &GoalSpec{WaitCustomerTimeout: "60", DeliveryCooldown: "60"},
		Loop:    &LoopSpec{Interval: "5", OnTimeout: "reboot", OnError: "panic", MaxIdleIterations: &negative},
	}
	f := File{Version: 1, Agents: map[string]AgentSpec{"a": a}, Groups: map[string]GroupSpec{"team": {}}}
	update := func(change func()) func() {
		return func() {
			change()
			f.Agents["a"] = a
		}
	}
	runValidationChain(t, &f, []validationStep{
		{`agent "a" has no image`, update(func() { a.Image = "i" })},
		{`agent "a" references invalid group name "Bad Group!"`, update(func() { a.Group = "ghost" })},
		{`agent "a" references undeclared group "ghost"`, update(func() { a.Group = "team" })},
		{`agent "a" budget: invalid budget mode "wat" (want warn|block|enforce)`, update(func() { a.Budget = nil })},
		{`agent "a" harness.type is required`, update(func() { a.Harness = nil })},
		{`agent "a" invalid timeout "60" (want a duration like 60m/2h/90s): time: missing unit in duration "60"`, update(func() { a.Timeout = "" })},
		{`agent "a" invalid goal wait_customer_timeout "60": time: missing unit in duration "60"`, update(func() { a.Goal.WaitCustomerTimeout = "" })},
		{`agent "a" invalid goal delivery_cooldown "60": time: missing unit in duration "60"`, update(func() { a.Goal.DeliveryCooldown = "" })},
		{`agent "a" loop invalid interval "5" (want a duration like 60m/2h/90s): time: missing unit in duration "5"`, update(func() { a.Loop.Interval = "" })},
		{`agent "a" loop on_timeout "reboot" must be restart|stop`, update(func() { a.Loop.OnTimeout = "" })},
		{`agent "a" loop on_error "panic" must be restart|stop`, update(func() { a.Loop.OnError = "" })},
		{`agent "a" loop max_idle_iterations -1 must be >= 0`, update(func() { a.Loop.MaxIdleIterations = nil })},
	})
}

func TestValidateTaskQueueChecksInOrder(t *testing.T) {
	t.Parallel()
	q := TaskQueueSpec{Workflow: "Bad Ref!", Pools: map[string][]string{"bad pool": {"ghost"}}}
	f := File{Version: 1, TaskQueues: map[string]TaskQueueSpec{"KEY": q}}
	update := func(change func()) func() {
		return func() {
			change()
			f.TaskQueues["KEY"] = q
		}
	}
	runValidationChain(t, &f, []validationStep{
		{`task queue "KEY" name is required`, update(func() { q.Name = "Keys" })},
		{`task queue "KEY" workflow "Bad Ref!": invalid workflow name "Bad Ref!"`, update(func() { q.Workflow = "" })},
		{`task queue "KEY" has unsafe pool route name "bad pool"`, update(func() { q.Pools = map[string][]string{"dev": {"ghost"}} })},
		{`task queue "KEY" pool "dev" references undeclared agent "ghost"`, update(func() { q.Pools = nil })},
	})
}
