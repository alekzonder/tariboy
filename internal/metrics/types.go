package metrics

import (
	"encoding/json"
	"net/http"

	"github.com/invopop/jsonschema"
)

// Error is a metrics request error with its HTTP status.
type Error struct {
	Status int
	Code   string
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func errInvalid(msg string) error {
	return &Error{Status: http.StatusBadRequest, Code: "invalid_param", Msg: msg}
}

// MetricsTokens are token counts by kind.
type MetricsTokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheWrite int64 `json:"cache_write"`
	CacheRead  int64 `json:"cache_read"`
}

func (t *MetricsTokens) add(o MetricsTokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheWrite += o.CacheWrite
	t.CacheRead += o.CacheRead
}

// MetricsUsage is the AI usage of a set of iterations.
type MetricsUsage struct {
	Iterations int64         `json:"iterations" jsonschema:"description=distinct iterations"`
	Tokens     MetricsTokens `json:"tokens"`
	CostUSD    float64       `json:"cost_usd" jsonschema:"description=US dollars"`
}

// MetricsNullSeconds is a whole number of seconds that encodes as null when not Valid.
type MetricsNullSeconds struct {
	Value int64
	Valid bool
}

func (s MetricsNullSeconds) MarshalJSON() ([]byte, error) {
	if !s.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(s.Value)
}

func (s *MetricsNullSeconds) UnmarshalJSON(b []byte) error {
	var v *int64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = MetricsNullSeconds{}
	if v != nil {
		*s = MetricsNullSeconds{Value: *v, Valid: true}
	}
	return nil
}

func (MetricsNullSeconds) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "integer", Description: "seconds, null when unknown", Extras: map[string]any{"nullable": true}}
}

// MetricsAxisScore is the average, minimum and number of scores of one assessment
// axis. It encodes as null when the axis has no score.
type MetricsAxisScore struct {
	Avg   float64
	Min   int64
	Count int64
}

type axisWire struct {
	Avg   float64 `json:"avg"`
	Min   int64   `json:"min"`
	Count int64   `json:"count"`
}

func (a MetricsAxisScore) MarshalJSON() ([]byte, error) {
	if a.Count == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(axisWire(a))
}

func (a *MetricsAxisScore) UnmarshalJSON(b []byte) error {
	var w *axisWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*a = MetricsAxisScore{}
	if w != nil {
		*a = MetricsAxisScore(*w)
	}
	return nil
}

func axisSchema() *jsonschema.Schema {
	props := jsonschema.NewProperties()
	props.Set("avg", &jsonschema.Schema{Type: "number"})
	props.Set("min", &jsonschema.Schema{Type: "integer"})
	props.Set("count", &jsonschema.Schema{Type: "integer"})
	return &jsonschema.Schema{Type: "object", Properties: props, Required: []string{"avg", "min", "count"},
		AdditionalProperties: jsonschema.FalseSchema, Extras: map[string]any{"nullable": true}}
}

func (MetricsAxisScore) JSONSchema() *jsonschema.Schema { return axisSchema() }

// MetricsQuality holds the scored assessment axes. It encodes as null when no axis
// has a score.
type MetricsQuality struct {
	ImageProcess MetricsAxisScore
	Role         MetricsAxisScore
	TaskFit      MetricsAxisScore
}

func (q MetricsQuality) MarshalJSON() ([]byte, error) {
	if q.ImageProcess.Count+q.Role.Count+q.TaskFit.Count == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(qualityWire(q))
}

type qualityWire struct {
	ImageProcess MetricsAxisScore `json:"image_process"`
	Role         MetricsAxisScore `json:"role"`
	TaskFit      MetricsAxisScore `json:"task_fit"`
}

func (q *MetricsQuality) UnmarshalJSON(b []byte) error {
	var w *qualityWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*q = MetricsQuality{}
	if w != nil {
		*q = MetricsQuality(*w)
	}
	return nil
}

func (MetricsQuality) JSONSchema() *jsonschema.Schema {
	props := jsonschema.NewProperties()
	for _, axis := range axes {
		props.Set(axis, axisSchema())
	}
	return &jsonschema.Schema{Type: "object", Properties: props, Required: axes,
		AdditionalProperties: jsonschema.FalseSchema, Extras: map[string]any{"nullable": true},
		Description: "average, minimum and count of scored assessment axes; null without scores"}
}

// MetricsReworkByOutcome counts the rework outcomes one by one.
type MetricsReworkByOutcome struct {
	ChangesRequested int64 `json:"changes_requested"`
	Rework           int64 `json:"rework"`
	Conflict         int64 `json:"conflict"`
}

// MetricsImageVersionUsage is the usage of the iterations that ran on one image
// version.
type MetricsImageVersionUsage struct {
	Image        string        `json:"image"`
	ImageRef     string        `json:"image_ref"`
	ImageVersion string        `json:"image_version" jsonschema:"description=unknown when no iterations row records it"`
	ImageDigest  string        `json:"image_digest"`
	Iterations   int64         `json:"iterations"`
	Tokens       MetricsTokens `json:"tokens"`
	CostUSD      float64       `json:"cost_usd"`
}

// MetricsTaskAgentUsage is one agent's share of a task.
type MetricsTaskAgentUsage struct {
	Agent  string                     `json:"agent"`
	Usage  MetricsUsage               `json:"usage"`
	Images []MetricsImageVersionUsage `json:"images"`
}

// MetricsWorkflowRef is a workflow image version a task ran under.
type MetricsWorkflowRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// TaskMetrics is the metrics of one task (metrics.task).
type TaskMetrics struct {
	Key               string                     `json:"key"`
	Title             string                     `json:"title"`
	Status            string                     `json:"status"`
	StartedAt         string                     `json:"started_at"`
	CompletedAt       string                     `json:"completed_at"`
	LeadTimeS         MetricsNullSeconds         `json:"lead_time_s"`
	StatusTimeS       map[string]int64           `json:"status_time_s" jsonschema:"description=seconds in each status"`
	Usage             MetricsUsage               `json:"usage"`
	ReworkRounds      int64                      `json:"rework_rounds"`
	ReworkByOutcome   MetricsReworkByOutcome     `json:"rework_by_outcome"`
	RejectedAdvances  int64                      `json:"rejected_advances"`
	CustomerQuestions int64                      `json:"customer_questions"`
	Quality           MetricsQuality             `json:"quality"`
	Agents            []MetricsTaskAgentUsage    `json:"agents"`
	Images            []MetricsImageVersionUsage `json:"images"`
	Workflows         []MetricsWorkflowRef       `json:"workflows"`
}

// MetricsTaskSums are the sums of the per-task metrics of a set of tasks.
type MetricsTaskSums struct {
	Iterations        int64         `json:"iterations"`
	Tokens            MetricsTokens `json:"tokens"`
	CostUSD           float64       `json:"cost_usd"`
	ReworkRounds      int64         `json:"rework_rounds"`
	RejectedAdvances  int64         `json:"rejected_advances"`
	CustomerQuestions int64         `json:"customer_questions"`
}

// MetricsTaskAverages are the per-task averages of a set of tasks.
type MetricsTaskAverages struct {
	Iterations        float64 `json:"iterations"`
	InputTokens       float64 `json:"input_tokens"`
	OutputTokens      float64 `json:"output_tokens"`
	CacheWriteTokens  float64 `json:"cache_write_tokens"`
	CacheReadTokens   float64 `json:"cache_read_tokens"`
	CostUSD           float64 `json:"cost_usd"`
	ReworkRounds      float64 `json:"rework_rounds"`
	RejectedAdvances  float64 `json:"rejected_advances"`
	CustomerQuestions float64 `json:"customer_questions"`
}

// MetricsTaskStats aggregates the tasks done in a period.
type MetricsTaskStats struct {
	Done            int                  `json:"tasks_done"`
	Mixed           int                  `json:"mixed_tasks" jsonschema:"description=tasks that ran on more than one version of this slice"`
	LeadTimeMedianS MetricsNullSeconds   `json:"lead_time_median_s"`
	LeadTimeAvgS    MetricsNullSeconds   `json:"lead_time_avg_s"`
	Sum             MetricsTaskSums      `json:"sum"`
	Avg             *MetricsTaskAverages `json:"avg,omitempty" jsonschema:"description=absent when no task is done"`
	Quality         MetricsQuality       `json:"quality"`
}

// MetricsSlice is the usage and the task aggregates of one agent, image version or
// workflow version, or of the whole server.
type MetricsSlice struct {
	Name    string           `json:"name,omitempty"`
	Version string           `json:"version,omitempty"`
	Usage   MetricsUsage     `json:"usage"`
	Tasks   MetricsTaskStats `json:"tasks"`
}

// MetricsReport is the server report over a period (metrics.server).
type MetricsReport struct {
	Since     string         `json:"since"`
	Until     string         `json:"until"`
	Total     MetricsSlice   `json:"total"`
	Agents    []MetricsSlice `json:"agents"`
	Images    []MetricsSlice `json:"images"`
	Workflows []MetricsSlice `json:"workflows"`
}
