// Package audit owns a per-agent append-only audit log at <agentRoot>/audit.jsonl.
//
// The daemon is the single writer. Each event is one JSON object per line:
// {seq, ts, type, source, iteration_id, data}. Writes are thread-safe and
// best-effort — a write failure is logged to stderr and swallowed, never
// propagated into the loop (the loop must never crash because logging failed).
// This mirrors the previous implementation's core/audit.py.
package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const tsFormat = "2006-01-02T15:04:05.000Z07:00"

// Event is one audit record. Source is one of {system, harness, shim}.
type Event struct {
	Seq         int64          `json:"seq"`
	TS          string         `json:"ts"`
	Type        string         `json:"type"`
	Source      string         `json:"source"`
	IterationID string         `json:"iteration_id,omitempty"`
	Data        map[string]any `json:"data,omitempty"`
}

// Log is a thread-safe, best-effort append-only writer for one agent.
type Log struct {
	path  string
	clock func() time.Time
	mu    sync.Mutex
	seq   int64
}

// Open returns a Log for path, seeding seq from the last line already present so
// seq stays monotonic across daemon restarts. clock defaults to time.Now.
func Open(path string, clock func() time.Time) *Log {
	if clock == nil {
		clock = time.Now
	}
	return &Log{path: path, clock: clock, seq: lastSeq(path)}
}

// Record appends one event and returns its seq. On any failure the seq is left
// unchanged and the error is reported to stderr (never returned to the caller).
func (l *Log) Record(typ, source, iterationID string, data map[string]any) int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	next := l.seq + 1
	ev := Event{
		Seq:         next,
		TS:          l.clock().UTC().Format(tsFormat),
		Type:        typ,
		Source:      source,
		IterationID: iterationID,
		Data:        data,
	}
	line, err := json.Marshal(ev)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit: marshal error: %v\n", err)
		return l.seq
	}
	if dir := filepath.Dir(l.path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o700)
	}
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit: open %s: %v\n", l.path, err)
		return l.seq
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "audit: write %s: %v\n", l.path, err)
		return l.seq
	}
	l.seq = next
	return next
}

// lastSeq returns the seq of the last parseable event in path, or 0.
func lastSeq(path string) int64 {
	evs, err := ReadEvents(path, 1, 0)
	if err != nil || len(evs) == 0 {
		return 0
	}
	return evs[len(evs)-1].Seq
}

// ReadEvents parses events from path. since keeps only seq > since; limit keeps
// only the last limit events (limit <= 0 = all). A missing file yields an empty
// slice and no error. Unparseable lines are skipped.
func ReadEvents(path string, limit int, since int64) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if since > 0 && ev.Seq <= since {
			continue
		}
		out = append(out, ev)
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// ReadByIteration returns all events whose IterationID matches, chronological.
// A missing file yields an empty slice and no error.
func ReadByIteration(path string, iterationID string) ([]Event, error) {
	all, err := ReadEvents(path, 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(all))
	for _, ev := range all {
		if ev.IterationID == iterationID {
			out = append(out, ev)
		}
	}
	return out, nil
}

// ReadBefore returns up to limit events with Seq < before, chronological (the
// scroll-back window). before <= 0 means "from the newest" (the last limit
// events overall). A non-positive limit returns all matching events.
func ReadBefore(path string, before int64, limit int) ([]Event, error) {
	all, err := ReadEvents(path, 0, 0)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(all))
	for _, ev := range all {
		if before <= 0 || ev.Seq < before {
			out = append(out, ev)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// searchText builds the case-folded haystack for a full-text query: the event's
// type, source, iteration id, and its data marshaled back to JSON, so a query
// matches any field of the whole record, not just one column.
func searchText(ev Event) string {
	var b strings.Builder
	b.WriteString(ev.Type)
	b.WriteByte(' ')
	b.WriteString(ev.Source)
	b.WriteByte(' ')
	b.WriteString(ev.IterationID)
	if ev.Data != nil {
		if d, err := json.Marshal(ev.Data); err == nil {
			b.WriteByte(' ')
			b.Write(d)
		}
	}
	return strings.ToLower(b.String())
}

// ReadFiltered returns events matching a TYPE set (event.Type equals any of
// types; an empty/all-blank set matches every type) AND a full-text query
// (case-insensitive substring across the whole record; empty query matches
// everything). The two conditions compose (AND). limit keeps only the newest
// limit matches (limit <= 0 = all). Results are chronological (oldest first),
// consistent with the other readers. A missing file yields an empty slice.
func ReadFiltered(path string, types []string, query string, limit int) ([]Event, error) {
	all, err := ReadEvents(path, 0, 0)
	if err != nil {
		return nil, err
	}
	typeSet := map[string]bool{}
	for _, t := range types {
		if t = strings.TrimSpace(t); t != "" {
			typeSet[t] = true
		}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Event, 0, len(all))
	for _, ev := range all {
		if len(typeSet) > 0 && !typeSet[ev.Type] {
			continue
		}
		if q != "" && !strings.Contains(searchText(ev), q) {
			continue
		}
		out = append(out, ev)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// DistinctTypes returns the sorted set of distinct event types present in the
// log — the "from real data" preset list for the audit-log type filter. A
// missing file yields an empty slice and no error.
func DistinctTypes(path string) ([]string, error) {
	all, err := ReadEvents(path, 0, 0)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []string{}
	for _, ev := range all {
		if ev.Type != "" && !seen[ev.Type] {
			seen[ev.Type] = true
			out = append(out, ev.Type)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Follow streams events from path: it waits for the file to appear, backfills
// existing events (applying since), then polls for appended lines every poll.
// The returned channel is closed when ctx is done.
func Follow(ctx context.Context, path string, since int64, poll time.Duration) <-chan Event {
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	ch := make(chan Event)
	go func() {
		defer close(ch)
		lastSeq := since
		emit := func() bool {
			evs, err := ReadEvents(path, 0, lastSeq)
			if err != nil {
				return true
			}
			for _, ev := range evs {
				select {
				case ch <- ev:
					lastSeq = ev.Seq
				case <-ctx.Done():
					return false
				}
			}
			return true
		}
		t := time.NewTicker(poll)
		defer t.Stop()
		for {
			if !emit() {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return ch
}
