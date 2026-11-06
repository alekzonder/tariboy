package compose

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// liveAgent is the part of a GET /api/agents row (agent.ps) the reconciler
// reads. The daemon serves the row as a JSON object; a contract test in
// internal/commands pins that every field below is present in it.
type liveAgent struct {
	Name                     string            `json:"name"`
	State                    string            `json:"state"`
	Image                    string            `json:"image"`
	Harness                  string            `json:"harness"`
	Enabled                  bool              `json:"enabled"`
	Group                    string            `json:"group"`
	Cwd                      string            `json:"cwd"`
	TimeoutS                 int               `json:"timeout_s"`
	IntervalS                int               `json:"interval_s"`
	OnTimeout                string            `json:"on_timeout"`
	OnError                  string            `json:"on_error"`
	MaxIdleIterations        int               `json:"max_idle_iterations"`
	GoalEnabled              bool              `json:"goal_enabled"`
	GoalWaitCustomerTimeoutS int               `json:"goal_wait_customer_timeout_s"`
	GoalDeliveryCooldownS    int               `json:"goal_delivery_cooldown_s"`
	Interactive              bool              `json:"interactive"`
	Env                      map[string]string `json:"env"`

	// View, Secrets and Retention are read per agent of the file, after the
	// list; agent.ps does not return them.
	View      liveAgentView   `json:"-"`
	Secrets   map[string]bool `json:"-"` // secret keys; the daemon never returns a value
	Retention *liveRetention  `json:"-"` // read only when the file sets retention
	// reprovisioned marks a row predicted after a re-provision, whose plugins
	// compose cannot predict.
	reprovisioned bool
}

// liveAgentView is the part of GET /api/agents/{name} (agent.inspect) the
// reconciler reads besides the agent.ps row.
type liveAgentView struct {
	Model              string   `json:"model"`
	Effort             string   `json:"effort"`
	Plugins            []string `json:"plugins"`
	HardTimeoutS       int      `json:"hard_timeout_s"`
	AIStallTimeoutS    int      `json:"ai_stall_timeout_s"`
	UserPrompt         string   `json:"user_prompt"`
	MessagesBatch      int      `json:"messages_batch"`
	MessagesMaxQueue   int      `json:"messages_max_queue"`
	MailInstantSenders []string `json:"mail_instant_senders"` // nil when not set
	Alias              string   `json:"alias"`
	Notes              string   `json:"notes"`
	Color              string   `json:"color"`
	Schedule           string   `json:"schedule"`
}

// liveRetention is the agent's own retention override.
type liveRetention struct {
	KeepIterations int   `json:"keep_iterations"`
	KeepDays       int   `json:"keep_days"`
	MaxBytes       int64 `json:"max_bytes"`
}

// liveGroup is the part of a GET /api/groups row the reconciler reads.
type liveGroup struct {
	Name string `json:"name"`
	Lead string `json:"lead"`
}

// liveBudget is the part of a GET /api/budgets row (budget.ls) the reconciler
// reads.
type liveBudget struct {
	Scope    string  `json:"scope"`
	LimitUSD float64 `json:"limit_usd"`
	PeriodS  int     `json:"period_s"`
	Mode     string  `json:"mode"`
}

// CheckAgentListContract reports whether a GET /api/agents response decodes
// into the rows the reconciler reads and every row carries every field it
// reads. A missing field would silently decode as its zero value and read as
// drift on every `up`. The daemon's agent.ps test calls it so the server and
// compose cannot drift apart.
func CheckAgentListContract(raw []byte) error {
	var rows struct {
		Agents []map[string]json.RawMessage `json:"agents"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return err
	}
	var typed struct {
		Agents []liveAgent `json:"agents"`
	}
	if err := json.Unmarshal(raw, &typed); err != nil {
		return fmt.Errorf("agent rows do not decode into the compose live view: %w", err)
	}
	for _, row := range rows.Agents {
		if err := checkFields[liveAgent](row); err != nil {
			return fmt.Errorf("agent row %s %w", row["name"], err)
		}
	}
	return nil
}

// CheckAgentViewContract is CheckAgentListContract for one GET
// /api/agents/{name} response; the daemon's agent.inspect test calls it.
func CheckAgentViewContract(raw []byte) error {
	var row map[string]json.RawMessage
	if err := json.Unmarshal(raw, &row); err != nil {
		return err
	}
	var typed liveAgentView
	if err := json.Unmarshal(raw, &typed); err != nil {
		return fmt.Errorf("agent view does not decode into the compose live view: %w", err)
	}
	if err := checkFields[liveAgentView](row); err != nil {
		return fmt.Errorf("agent view %w", err)
	}
	return nil
}

// checkFields reports the first JSON field of T that row lacks. Fields
// without a JSON name are not read from the response.
func checkFields[T any](row map[string]json.RawMessage) error {
	fields := reflect.TypeFor[T]()
	for i := range fields.NumField() {
		tag, _, _ := strings.Cut(fields.Field(i).Tag.Get("json"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		if _, ok := row[tag]; !ok {
			return fmt.Errorf("lacks field %q that compose reads", tag)
		}
	}
	return nil
}

// readAgentDetails reads what agent.ps does not return for an agent of the
// file: its agent.inspect view, its secret keys and, when the file sets
// retention, its retention override.
func (r *Runner) readAgentDetails(name string, a AgentSpec, cur *liveAgent) error {
	if err := r.get("/api/agents/"+name, &cur.View); err != nil {
		return err
	}
	var secrets struct {
		Keys []string `json:"keys"`
	}
	if err := r.get("/api/agents/"+name+"/secrets", &secrets); err != nil {
		return fmt.Errorf("secrets: %w", err)
	}
	cur.Secrets = map[string]bool{}
	for _, key := range secrets.Keys {
		cur.Secrets[key] = true
	}
	if a.Retention == nil {
		return nil
	}
	var retention struct {
		Override liveRetention `json:"override"`
	}
	if err := r.get("/api/agents/"+name+"/retention", &retention); err != nil {
		return fmt.Errorf("retention: %w", err)
	}
	cur.Retention = &retention.Override
	return nil
}

// liveAgents returns name -> row for every agent the daemon knows.
func (r *Runner) liveAgents() (map[string]liveAgent, error) {
	var env struct {
		Agents []liveAgent `json:"agents"`
	}
	if err := r.get("/api/agents", &env); err != nil {
		return nil, err
	}
	out := make(map[string]liveAgent, len(env.Agents))
	for _, a := range env.Agents {
		out[a.Name] = a
	}
	return out, nil
}

// liveGroups returns name -> row for every group the daemon knows.
func (r *Runner) liveGroups() (map[string]liveGroup, error) {
	var env struct {
		Groups []liveGroup `json:"groups"`
	}
	if err := r.get("/api/groups", &env); err != nil {
		return nil, err
	}
	out := make(map[string]liveGroup, len(env.Groups))
	for _, g := range env.Groups {
		out[g.Name] = g
	}
	return out, nil
}

// liveBudgets returns scope -> row for every budget the daemon knows.
func (r *Runner) liveBudgets() (map[string]liveBudget, error) {
	var env struct {
		Budgets []liveBudget `json:"budgets"`
	}
	if err := r.get("/api/budgets", &env); err != nil {
		return nil, err
	}
	out := make(map[string]liveBudget, len(env.Budgets))
	for _, b := range env.Budgets {
		out[b.Scope] = b
	}
	return out, nil
}

// pendingImage returns the image ref an agent adopts on its next iteration,
// or "" when none is pending.
func (r *Runner) pendingImage(name string) (string, error) {
	var status struct {
		Pending struct {
			Ref string `json:"ref"`
		} `json:"pending"`
	}
	if err := r.get("/api/agents/"+name+"/image", &status); err != nil {
		return "", err
	}
	return status.Pending.Ref, nil
}

func (r *Runner) get(route string, v any) error {
	raw, err := r.call.Call("GET", route, map[string]string{})
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

// normalizeImageRef spells an image ref the way the daemon stores it: a bare
// name means NAME:latest.
func normalizeImageRef(ref string) string {
	if ref == "" || strings.Contains(ref, ":") {
		return ref
	}
	return ref + ":latest"
}
