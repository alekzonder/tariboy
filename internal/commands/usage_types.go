package commands

// Response types of the usage, budget, logs and iteration commands. Each
// command declares its type with registry.WithResult, which derives the
// OpenAPI result schema from it.

// UsageRow is the AI usage of one agent, image and request-time group.
type UsageRow struct {
	Agent            string  `json:"agent"`
	Image            string  `json:"image"`
	GroupID          string  `json:"group_id"`
	GroupName        string  `json:"group_name"`
	Requests         int     `json:"requests"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// UsageSeriesPoint is the AI usage within one time bucket.
type UsageSeriesPoint struct {
	BucketStart string  `json:"bucket_start"`
	Requests    int     `json:"requests"`
	Tokens      int     `json:"tokens"`
	CostUSD     float64 `json:"cost_usd"`
}

// UsageRequest is one recorded AI request.
type UsageRequest struct {
	ID               string  `json:"id"`
	TS               string  `json:"ts"`
	Agent            string  `json:"agent"`
	Image            string  `json:"image"`
	Provider         string  `json:"provider"`
	Model            string  `json:"model"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Status           string  `json:"status"`
	GroupID          string  `json:"group_id"`
	GroupName        string  `json:"group_name"`
}

// UsageReport is the aggregated AI usage with its totals, a daily series
// and the newest requests (usage).
type UsageReport struct {
	Rows                  []UsageRow         `json:"rows"`
	Count                 int                `json:"count"`
	TotalRequests         int                `json:"total_requests"`
	TotalCostUSD          float64            `json:"total_cost_usd"`
	TotalInputTokens      int                `json:"total_input_tokens"`
	TotalOutputTokens     int                `json:"total_output_tokens"`
	TotalCacheWriteTokens int                `json:"total_cache_write_tokens"`
	TotalCacheReadTokens  int                `json:"total_cache_read_tokens"`
	Series                []UsageSeriesPoint `json:"series"`
	Requests              []UsageRequest     `json:"requests"`
}

// AgentUsageTotals sums an agent's AI usage over the requested window.
type AgentUsageTotals struct {
	Requests         int     `json:"requests"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// AgentUsageRow is an agent's AI usage for one grouping key. Title appears
// when the rows are grouped by task or epic.
type AgentUsageRow struct {
	Key              string  `json:"key"`
	Title            string  `json:"title,omitempty"`
	Requests         int     `json:"requests"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// AgentUsage is one agent's grouped AI usage and its time series
// (agent.usage).
type AgentUsage struct {
	Agent   string             `json:"agent"`
	GroupBy string             `json:"group_by"`
	Bucket  string             `json:"bucket"`
	Totals  AgentUsageTotals   `json:"totals"`
	Rows    []AgentUsageRow    `json:"rows"`
	Series  []UsageSeriesPoint `json:"series"`
}

// BudgetLimit is one configured cost budget.
type BudgetLimit struct {
	Scope    string  `json:"scope"`
	LimitUSD float64 `json:"limit_usd"`
	PeriodS  int     `json:"period_s"`
	Mode     string  `json:"mode"`
}

// BudgetList is the configured cost budgets (budget.ls).
type BudgetList struct {
	Budgets []BudgetLimit `json:"budgets"`
	Count   int           `json:"count"`
}

// BudgetSpend is a budget's spend over its current rolling period.
type BudgetSpend struct {
	Scope    string  `json:"scope"`
	LimitUSD float64 `json:"limit_usd"`
	SpentUSD float64 `json:"spent_usd"`
	Mode     string  `json:"mode"`
	Over     bool    `json:"over"`
}

// BudgetStatusList is the spend of every budget (budget.status).
type BudgetStatusList struct {
	Budgets []BudgetSpend `json:"budgets"`
	Count   int           `json:"count"`
}

// AuditLogEvent is one event of an agent's audit log; Data is the event
// data encoded as a JSON object string.
type AuditLogEvent struct {
	Seq         int64  `json:"seq"`
	Kind        string `json:"kind"`
	Source      string `json:"source"`
	IterationID string `json:"iteration_id"`
	Data        string `json:"data"`
	At          string `json:"at"`
}

// AuditLogResult is the logs result: the event types present for
// distinct=types, otherwise a page of events. Capped appears for a type or
// full-text filter and reports a result cut at its limit.
type AuditLogResult struct {
	Types  []string        `json:"types,omitzero"`
	Events []AuditLogEvent `json:"events,omitzero"`
	Count  *int            `json:"count,omitempty"`
	Capped *bool           `json:"capped,omitempty"`
}

// newAuditLogPage returns a page of events; capped is nil unless a filter
// limited the page.
func newAuditLogPage(events []AuditLogEvent, capped *bool) AuditLogResult {
	count := len(events)
	return AuditLogResult{Events: events, Count: &count, Capped: capped}
}

// IterationTimeoutResult is an iteration's soft timeout after an extension
// (iteration.extend-timeout).
type IterationTimeoutResult struct {
	ID                  string `json:"id"`
	TimeoutDeadline     string `json:"timeout_deadline"`
	HardTimeoutDeadline string `json:"hard_timeout_deadline"`
	TimeoutExtensions   int    `json:"timeout_extensions"`
	ShimSync            string `json:"shim_sync"`
}

// IterationRow is one iteration in an agent's iteration list.
type IterationRow struct {
	ID         string   `json:"id"`
	Trigger    string   `json:"trigger"`
	Status     string   `json:"status"`
	StartedAt  string   `json:"started_at"`
	Done       bool     `json:"done"`
	Productive bool     `json:"productive"`
	Tags       []string `json:"tags"`
}

// IterationList is an agent's iterations (iteration.ls).
type IterationList struct {
	Iterations []IterationRow `json:"iterations"`
	Count      int            `json:"count"`
}

// IterationTagResult is the resulting tags of each changed iteration, keyed
// by iteration id (iteration.tag.add, iteration.tag.rm, iteration.tag.set).
type IterationTagResult struct {
	Tags  map[string][]string `json:"tags"`
	Count int                 `json:"count"`
}

// IterationView is one iteration (iteration.inspect). The exit code and
// resource use appear once the harness reported them.
type IterationView struct {
	ID                   string   `json:"id"`
	Agent                string   `json:"agent"`
	Trigger              string   `json:"trigger"`
	Status               string   `json:"status"`
	StartedAt            string   `json:"started_at"`
	EndedAt              string   `json:"ended_at"`
	Done                 bool     `json:"done"`
	Productive           bool     `json:"productive"`
	PromptPath           string   `json:"prompt_path"`
	ImageRef             string   `json:"image_ref"`
	ImageVersion         string   `json:"image_version"`
	ImageDigest          string   `json:"image_digest"`
	PromptTemplateSHA256 string   `json:"prompt_template_sha256"`
	Tags                 []string `json:"tags"`
	ExitCode             *int     `json:"exit_code,omitempty"`
	CPUMs                *int     `json:"cpu_ms,omitempty"`
	MemPeakKB            *int     `json:"mem_peak_kb,omitempty"`
}

// IterationLogs is an iteration's harness output; empty when none was
// written (iteration.logs).
type IterationLogs struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}
