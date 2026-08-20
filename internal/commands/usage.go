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

func usageCommand() registry.Command {
	return registry.Command{
		Path:    "usage",
		Summary: "Aggregate AI usage and cost from ai_requests",
		Args: []registry.Arg{
			{Name: "agent", Flag: "agent", Short: "a", Type: registry.String, Help: "filter by agent"},
			{Name: "image", Flag: "image", Short: "i", Type: registry.String, Help: "filter by image name"},
			{Name: "since", Flag: "since", Short: "s", Type: registry.String, Help: "RFC3339 lower bound"},
			{Name: "until", Flag: "until", Short: "u", Type: registry.String, Help: "RFC3339 upper bound"},
			{Name: "group", Flag: "group", Short: "g", Type: registry.String, Help: "filter by request-time group snapshot; use __ungrouped__ for no group"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/usage"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			usage := aiproxy.NewStore(c.Store, nil)
			f := aiproxy.UsageFilter{
				Agent: str(p, "agent"), Image: str(p, "image"), Since: str(p, "since"), Until: str(p, "until"),
				Group: str(p, "group"),
			}
			rows, series, requests, err := usage.Report(f, "1d", 200)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(rows))
			outSeries := make([]map[string]any, 0, len(series))
			outRequests := make([]map[string]any, 0, len(requests))
			var totCost float64
			var totReq, totIn, totOut, totCW, totCR int
			for _, r := range rows {
				out = append(out, map[string]any{"agent": r.Agent, "image": r.ImageName,
					"group_id": r.GroupID, "group_name": r.GroupName,
					"requests": r.Requests, "input_tokens": r.InputTokens, "output_tokens": r.OutputTokens,
					"cache_write_tokens": r.CacheWriteTokens, "cache_read_tokens": r.CacheReadTokens,
					"cost_usd": r.CostUSD})
				totCost += r.CostUSD
				totReq += r.Requests
				totIn += r.InputTokens
				totOut += r.OutputTokens
				totCW += r.CacheWriteTokens
				totCR += r.CacheReadTokens
			}
			for _, r := range series {
				outSeries = append(outSeries, map[string]any{
					"bucket_start": r.BucketStart, "requests": r.Requests, "tokens": r.Tokens, "cost_usd": r.CostUSD,
				})
			}
			for _, r := range requests {
				outRequests = append(outRequests, map[string]any{
					"id": r.ID, "ts": r.TS, "agent": r.Agent, "image": r.ImageName,
					"provider": r.Provider, "model": r.Model, "input_tokens": r.InputTokens,
					"output_tokens": r.OutputTokens, "cache_write_tokens": r.CacheWriteTokens,
					"cache_read_tokens": r.CacheReadTokens, "cost_usd": r.CostUSD, "status": r.Status,
					"group_id": r.GroupID, "group_name": r.GroupName,
				})
			}
			return map[string]any{"rows": out, "count": len(out),
				"total_requests": totReq, "total_cost_usd": math.Round(totCost*1e6) / 1e6,
				"total_input_tokens": totIn, "total_output_tokens": totOut,
				"total_cache_write_tokens": totCW, "total_cache_read_tokens": totCR,
				"series": outSeries, "requests": outRequests}, nil
		},
	}
}

// agentUsage is the per-agent Usage tab's backend (epic dev-t-3e1 §3): grouped
// rows + a time-bucketed series over one agent's ai_requests, scoped by an
// optional RFC3339 window.
func agentUsage() registry.Command {
	return registry.Command{
		Path:    "agent.usage",
		Summary: "Per-agent AI usage: grouped rows + time-bucketed series",
		Args: []registry.Arg{
			{Name: "name", Type: registry.String, Required: true, Help: "agent name"},
			{Name: "since", Flag: "since", Short: "s", Type: registry.String, Help: "RFC3339 lower bound (inclusive)"},
			{Name: "until", Flag: "until", Short: "u", Type: registry.String, Help: "RFC3339 upper bound (exclusive)"},
			{Name: "group_by", Flag: "group-by", Short: "g", Type: registry.String, Help: "iteration|task|epic|model (default iteration)"},
			{Name: "bucket", Flag: "bucket", Short: "b", Type: registry.String, Help: "series bucket 5m|15m|1h|1d (default 1h)"},
		},
		HTTP: &registry.HTTPRoute{Method: "GET", Path: "/api/agents/{name}/usage"},
		Handler: func(c *registry.Ctx, p registry.Params) (any, error) {
			a, err := getAgent(c, str(p, "name"))
			if err != nil {
				return nil, err // api.UserError not_found
			}
			groupBy := str(p, "group_by")
			if groupBy == "" {
				groupBy = "iteration"
			}
			bucket := str(p, "bucket")
			if bucket == "" {
				bucket = "1h"
			}

			st := aiproxy.NewStore(c.Store, nil)
			f := aiproxy.UsageFilter{Agent: a.Name, Since: str(p, "since"), Until: str(p, "until")}
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

			outRows := make([]map[string]any, 0, len(rows))
			var totReq, totIn, totOut, totCW, totCR int
			var totCost float64
			for _, r := range rows {
				key := r.Key
				if key == "" {
					key = "untagged"
				}
				row := map[string]any{
					"key": key, "requests": r.Requests,
					"input_tokens": r.InputTokens, "output_tokens": r.OutputTokens,
					"cache_write_tokens": r.CacheWriteTokens, "cache_read_tokens": r.CacheReadTokens,
					"cost_usd": r.CostUSD,
				}
				if groupBy == "task" || groupBy == "epic" {
					switch {
					case r.Key == "":
						row["title"] = "untagged"
					case titles[r.Key] != "":
						row["title"] = titles[r.Key]
					default:
						row["title"] = r.Key // unknown/failed → bare id
					}
				}
				outRows = append(outRows, row)
				totReq += r.Requests
				totIn += r.InputTokens
				totOut += r.OutputTokens
				totCW += r.CacheWriteTokens
				totCR += r.CacheReadTokens
				totCost += r.CostUSD
			}

			outSeries := make([]map[string]any, 0, len(series))
			for _, sr := range series {
				outSeries = append(outSeries, map[string]any{
					"bucket_start": sr.BucketStart, "requests": sr.Requests,
					"tokens": sr.Tokens, "cost_usd": sr.CostUSD,
				})
			}

			return map[string]any{
				"agent": a.Name, "group_by": groupBy, "bucket": bucket,
				"totals": map[string]any{
					"requests": totReq, "input_tokens": totIn, "output_tokens": totOut,
					"cache_write_tokens": totCW, "cache_read_tokens": totCR,
					"cost_usd": math.Round(totCost*1e6) / 1e6,
				},
				"rows": outRows, "series": outSeries,
			}, nil
		},
	}
}
