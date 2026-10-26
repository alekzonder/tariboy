package mail_test

import (
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

// fakeDirectory is the Directory the tests control: agents with their inbox
// caps, installed plugins, queues and groups.
type fakeDirectory struct {
	agents   map[string]int // name -> messages_max_queue
	customer string
	plugins  map[string]bool
	queues   map[string]bool
	groups   map[string][]string
}

func newDirectory() *fakeDirectory {
	return &fakeDirectory{
		agents:   map[string]int{"alice": 100, "bob": 100, "carol": 100},
		customer: "customer",
		plugins:  map[string]bool{"telegram": true},
		queues:   map[string]bool{"OPS": true},
		groups:   map[string][]string{"dev": {"alice", "bob", "carol"}, "empty": {"alice"}},
	}
}

func (d *fakeDirectory) AgentExists(name string) (bool, error) {
	_, ok := d.agents[name]
	return ok, nil
}

func (d *fakeDirectory) InboxCap(agent string) (int, error) { return d.agents[agent], nil }

func (d *fakeDirectory) CustomerLogin() string { return d.customer }

func (d *fakeDirectory) PluginInstalled(name string) (bool, error) { return d.plugins[name], nil }

func (d *fakeDirectory) QueueExists(prefix string) (bool, error) { return d.queues[prefix], nil }

func (d *fakeDirectory) GroupMembers(group string) ([]string, bool, error) {
	m, ok := d.groups[group]
	return m, ok, nil
}

// recorder captures every hook the mail service fires.
type recorder struct {
	mu      sync.Mutex
	wakes   [][]string
	plugins [][]string
	hints   []mail.Hint
	audits  []string // "agent kind"
}

func (r *recorder) wake(agents []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wakes = append(r.wakes, append([]string(nil), agents...))
}

func (r *recorder) pluginWake(plugins []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = append(r.plugins, append([]string(nil), plugins...))
}

func (r *recorder) hint(h mail.Hint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hints = append(r.hints, h)
}

func (r *recorder) audit(agent, kind, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.audits = append(r.audits, agent+" "+kind)
}

func (r *recorder) woken() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, w := range r.wakes {
		out = append(out, w...)
	}
	sort.Strings(out)
	return out
}

func (r *recorder) auditKinds(kind string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, a := range r.audits {
		if len(a) >= len(kind) && a[len(a)-len(kind):] == kind {
			n++
		}
	}
	return n
}

type fixture struct {
	st  *store.Store
	m   *mail.Mail
	dir *fakeDirectory
	rec *recorder
	now *time.Time
}

// newFixture opens a migrated store and a Mail whose clock moves one second
// per call, so created_at orders letters in send order.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	st := storetest.Open(t)
	now := time.Date(2026, 10, 26, 10, 42, 0, 0, time.UTC)
	clock := func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	dir := newDirectory()
	m := mail.New(st, clock, dir)
	rec := &recorder{}
	m.SetWakeHook(rec.wake)
	m.SetPluginWakeHook(rec.pluginWake)
	m.SetEventHook(rec.hint)
	m.SetAuditHook(rec.audit)
	return &fixture{st: st, m: m, dir: dir, rec: rec, now: &now}
}

func (f *fixture) send(t *testing.T, l mail.Letter) []mail.Letter {
	t.Helper()
	out, err := f.m.Send(l)
	if err != nil {
		t.Fatalf("send %+v: %v", l, err)
	}
	return out
}

func (f *fixture) sendOne(t *testing.T, l mail.Letter) mail.Letter {
	t.Helper()
	out := f.send(t, l)
	if len(out) != 1 {
		t.Fatalf("send wrote %d letters; want 1", len(out))
	}
	return out[0]
}

func (f *fixture) show(t *testing.T, as, id string) mail.Letter {
	t.Helper()
	l, _, err := f.m.Show(as, id)
	if err != nil {
		t.Fatalf("show %s: %v", id, err)
	}
	return l
}

func ids(letters []mail.Letter) []string {
	out := make([]string, len(letters))
	for i, l := range letters {
		out[i] = l.ID
	}
	return out
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if got := mail.ErrorCode(err); got != code {
		t.Fatalf("error = %v (code %q); want code %q", err, got, code)
	}
}
