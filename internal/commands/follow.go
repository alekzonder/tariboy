package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"time"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/registry"
)

// followAgentLogs is the CLI-local `logs -f` composite: it polls the agent's
// logs endpoint (backed by the per-agent audit.jsonl) every tailPollInterval,
// printing events newer than the last seq it has seen, until ctx is cancelled.
// Polling the file source (rather than the SSE hub) means follow shows the same
// full audit stream — control-plane, shim, and harness output — as non-follow.
// It runs in the CLI process, so it needs no Store.
func followAgentLogs(ctx context.Context, sock string, p registry.Params, out io.Writer) error {
	c := client.New(sock)
	route := "/api/agents/" + url.PathEscape(str(p, "name")) + "/logs"
	var last int64
	ticker := time.NewTicker(tailPollInterval)
	defer ticker.Stop()
	for {
		raw, err := c.Call("GET", route, map[string]string{"since": strconv.FormatInt(last, 10)})
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return api.UserError{Code: "stream_failed", Msg: err.Error()}
		}
		var res struct {
			Events []struct {
				Seq  int64  `json:"seq"`
				Kind string `json:"kind"`
				Data string `json:"data"`
				At   string `json:"at"`
			} `json:"events"`
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return err
		}
		for _, e := range res.Events {
			fmt.Fprintf(out, "[%s] %s %s\n", e.At, e.Kind, e.Data)
			if e.Seq > last {
				last = e.Seq
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// tailPollInterval is how often `channel tail -f` re-queries for new messages.
var tailPollInterval = 500 * time.Millisecond

// followChannelTail is the CLI-local `channel tail -f` composite: it polls the
// channel's messages endpoint every tailPollInterval, printing rows newer than
// the last id it has seen (via ?since=<id>), until ctx is cancelled. The first
// poll prints the recent backlog; subsequent polls fetch only new messages.
func followChannelTail(ctx context.Context, sock string, p registry.Params, out io.Writer) error {
	c := client.New(sock)
	route := "/api/channels/" + url.PathEscape(str(p, "channel")) + "/messages"
	last := str(p, "since")
	ticker := time.NewTicker(tailPollInterval)
	defer ticker.Stop()
	for {
		q := map[string]string{}
		if last != "" {
			q["since"] = last
		}
		raw, err := c.Call("GET", route, q)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		var res struct {
			Messages []struct {
				ID     string `json:"id"`
				Type   string `json:"type"`
				Source string `json:"source"`
				Text   string `json:"text"`
			} `json:"messages"`
		}
		if err := json.Unmarshal(raw, &res); err != nil {
			return err
		}
		for _, m := range res.Messages {
			fmt.Fprintf(out, "[%s] %s %s: %s\n", m.ID, m.Type, m.Source, m.Text)
			last = m.ID
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
