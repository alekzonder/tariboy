package commands

import (
	"context"
	"errors"
	"math"
	"strconv"

	"github.com/alekzonder/tariboy/internal/aiproxy"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
	"github.com/alekzonder/tariboy/internal/tasks"
)

func parseFloat(s string) (float64, error) { return strconv.ParseFloat(s, 64) }

// usageWindow validates the optional since and until bounds of a usage query
// and returns them in the stored time format.
func usageWindow(p registry.Params) (since, until string, err error) {
	if since, err = activityBound(p.String("since")); err != nil {
		return "", "", err
	}
	if until, err = activityBound(p.String("until")); err != nil {
		return "", "", err
	}
	return since, until, nil
}

func usageCommand() registry.Command {
	return registry.WithResult[UsageReport](registry.Command{
		Path:    "usage",
		Summary: "Aggregate AI usage and cost from ai_requests",
		Args: []registry.Arg{
			{Name: "agent", Flag: "agent", Short: "a", Type: registry.String, Help: "filter by agent"},
			{Name: "image", Flag: "image", Short: "i", Type: registry.String, Help: "filter by image name"},
			{Name: "since", Flag: "since", Short: "s", Type: registry.String, Help: "RFC 3339 time (lower bound)"},
			{Name: "until", Flag: "until", Short: "u", Type: registry.String, Help: "RFC 3339 time (upper bound)"},
			{Name: "group", Flag: "group", Short: "g", Type: registry.String, Help: "filter by request-time group snapshot; use __ungrouped__ for no group"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/usage"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			since, until, err := usageWindow(p.Params)
			if err != nil {
				return nil, err
			}
			usage := aiproxy.NewStore(c.Store, nil)
			f := aiproxy.UsageFilter{
				Agent: p.String("agent"), Image: p.String("image"), Since: since, Until: until,
				Group: p.String("group"),
			}
			rows, series, requests, err := usage.Report(f, "1d", 200)
			if err != nil {
				return nil, err
			}
			out := UsageReport{Rows: make([]UsageRow, 0, len(rows)),
				Series: usageSeries(series), Requests: make([]UsageRequest, 0, len(requests))}
			var totCost float64
			for _, r := range rows {
				out.Rows = append(out.Rows, UsageRow{Agent: r.Agent, Image: r.ImageName,
					GroupID: r.GroupID, GroupName: r.GroupName,
					Requests: r.Requests, InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
					CacheWriteTokens: r.CacheWriteTokens, CacheReadTokens: r.CacheReadTokens,
					CostUSD: r.CostUSD})
				totCost += r.CostUSD
				out.TotalRequests += r.Requests
				out.TotalInputTokens += r.InputTokens
				out.TotalOutputTokens += r.OutputTokens
				out.TotalCacheWriteTokens += r.CacheWriteTokens
				out.TotalCacheReadTokens += r.CacheReadTokens
			}
			for _, r := range requests {
				out.Requests = append(out.Requests, UsageRequest{
					ID: r.ID, TS: r.TS, Agent: r.Agent, Image: r.ImageName,
					Provider: r.Provider, Model: r.Model, InputTokens: r.InputTokens,
					OutputTokens: r.OutputTokens, CacheWriteTokens: r.CacheWriteTokens,
					CacheReadTokens: r.CacheReadTokens, CostUSD: r.CostUSD, Status: r.Status,
					GroupID: r.GroupID, GroupName: r.GroupName,
				})
			}
			out.Count = len(out.Rows)
			out.TotalCostUSD = math.Round(totCost*1e6) / 1e6
			return out, nil
		},
	})
}

// usageSeries converts the stored time buckets into response points.
func usageSeries(series []aiproxy.SeriesRow) []UsageSeriesPoint {
	out := make([]UsageSeriesPoint, 0, len(series))
	for _, r := range series {
		out = append(out, UsageSeriesPoint{BucketStart: r.BucketStart, Requests: r.Requests, Tokens: r.Tokens, CostUSD: r.CostUSD})
	}
	return out
}

// agentUsage is the per-agent Usage tab's backend (epic dev-t-3e1 §3): grouped
// rows + a time-bucketed series over one agent's ai_requests, scoped by an
// optional RFC 3339 window.
func agentUsage() registry.Command {
	return registry.WithResult[AgentUsage](registry.Command{
		Path:    "agent.usage",
		Summary: "Per-agent AI usage: grouped rows + time-bucketed series",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "since", Flag: "since", Short: "s", Type: registry.String, Help: "RFC 3339 time (inclusive lower bound)"},
			{Name: "until", Flag: "until", Short: "u", Type: registry.String, Help: "RFC 3339 time (exclusive upper bound)"},
			{Name: "group_by", Flag: "group-by", Short: "g", Type: registry.String, Help: "iteration|task|epic|model (default iteration)"},
			{Name: "bucket", Flag: "bucket", Short: "b", Type: registry.String, Help: "series bucket 5m|15m|1h|1d (default 1h)"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/usage"},
		Handler: func(c *registry.Ctx, p registry.Request) (any, error) {
			a, err := getAgent(c, p.String("name"))
			if err != nil {
				return nil, err // api.UserError not_found
			}
			groupBy := p.String("group_by")
			if groupBy == "" {
				groupBy = "iteration"
			}
			bucket := p.String("bucket")
			if bucket == "" {
				bucket = "1h"
			}

			since, until, err := usageWindow(p.Params)
			if err != nil {
				return nil, err
			}
			st := aiproxy.NewStore(c.Store, nil)
			f := aiproxy.UsageFilter{Agent: a.Name, Since: since, Until: until}
			rows, err := st.AggregateBy(f, groupBy)
			if err != nil {
				if errors.Is(err, aiproxy.ErrBadGroupBy) {
					return nil, api.UserError{Code: "bad_group_by", Msg: "group_by must be one of iteration|task|epic|model"}
				}
				return nil, err
			}
			series, err := st.Series(f, bucket)
			if err != nil {
				if errors.Is(err, aiproxy.ErrBadBucket) {
					return nil, api.UserError{Code: "bad_bucket", Msg: "bucket must be one of 5m|15m|1h|1d"}
				}
				return nil, err
			}

			// task/epic rows carry human titles. Resolve each distinct key through
			// Native Tasks as the agent; unavailable, unknown, or inaccessible keys
			// degrade to their bare id.
			titles := map[string]string{}
			if (groupBy == "task" || groupBy == "epic") && c.Tasks != nil {
				ids := map[string]struct{}{}
				for _, r := range rows {
					if r.Key != "" {
						ids[r.Key] = struct{}{}
					}
				}
				for id := range ids {
					detail, err := c.Tasks.GetTask(context.Background(), tasks.AgentActor(a.Name), id)
					if err == nil && detail.Task.Title != "" {
						titles[id] = detail.Task.Title
					}
				}
			}

			out := AgentUsage{Agent: a.Name, GroupBy: groupBy, Bucket: bucket,
				Rows: make([]AgentUsageRow, 0, len(rows)), Series: usageSeries(series)}
			var totCost float64
			for _, r := range rows {
				key := r.Key
				if key == "" {
					key = "untagged"
				}
				row := AgentUsageRow{
					Key: key, Requests: r.Requests,
					InputTokens: r.InputTokens, OutputTokens: r.OutputTokens,
					CacheWriteTokens: r.CacheWriteTokens, CacheReadTokens: r.CacheReadTokens,
					CostUSD: r.CostUSD,
				}
				if groupBy == "task" || groupBy == "epic" {
					switch {
					case r.Key == "":
						row.Title = "untagged"
					case titles[r.Key] != "":
						row.Title = titles[r.Key]
					default:
						row.Title = r.Key // unknown/failed → bare id
					}
				}
				out.Rows = append(out.Rows, row)
				out.Totals.Requests += r.Requests
				out.Totals.InputTokens += r.InputTokens
				out.Totals.OutputTokens += r.OutputTokens
				out.Totals.CacheWriteTokens += r.CacheWriteTokens
				out.Totals.CacheReadTokens += r.CacheReadTokens
				totCost += r.CostUSD
			}
			out.Totals.CostUSD = math.Round(totCost*1e6) / 1e6
			return out, nil
		},
	})
}
