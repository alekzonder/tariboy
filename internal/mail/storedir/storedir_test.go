package storedir

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/mail"
	"github.com/alekzonder/tariboy/internal/store/storetest"
)

func TestDirectoryReadsTheDaemonTables(t *testing.T) {
	st := storetest.Open(t)
	as := agent.NewStore(st)
	for _, a := range []agent.Agent{{Name: "lead", ImageRef: "i:latest", Group: "dev"}, {Name: "solo", ImageRef: "i:latest"}} {
		if err := as.Create(a); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`INSERT INTO groups(name, lead) VALUES ('dev', 'lead')`,
		`INSERT INTO groups(name, lead) VALUES ('empty', '')`,
		`INSERT INTO plugins(name) VALUES ('telegram')`,
		`INSERT INTO task_queues(prefix, name, created_at, updated_at) VALUES ('OPS', 'Ops', 'x', 'x')`,
	} {
		if _, err := st.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	d := New(st, "customer")
	if ok, err := d.AgentExists("lead"); err != nil || !ok {
		t.Fatalf("lead exists = %v, %v", ok, err)
	}
	if ok, err := d.AgentExists("ghost"); err != nil || ok {
		t.Fatalf("ghost exists = %v, %v", ok, err)
	}
	if n, err := d.InboxCap("solo"); err != nil || n != 100 {
		t.Fatalf("solo inbox cap = %d, %v; want the default 100", n, err)
	}
	if ok, _ := d.PluginInstalled("telegram"); !ok {
		t.Fatal("telegram is installed")
	}
	if ok, _ := d.QueueExists("OPS"); !ok {
		t.Fatal("queue OPS exists")
	}
	if ok, _ := d.QueueExists("NOPE"); ok {
		t.Fatal("queue NOPE does not exist")
	}
	if members, ok, err := d.GroupMembers("dev"); err != nil || !ok || len(members) != 1 || members[0] != "lead" {
		t.Fatalf("dev = %v, %v, %v", members, ok, err)
	}
	if members, ok, _ := d.GroupMembers("empty"); !ok || len(members) != 0 {
		t.Fatalf("empty = %v, %v", members, ok)
	}
	if _, ok, _ := d.GroupMembers("ghost"); ok {
		t.Fatal("group ghost does not exist")
	}
}

func TestPluginReceivesMailNeedsEnabledSink(t *testing.T) {
	st := storetest.Open(t)
	for _, q := range []string{
		`INSERT INTO plugins(name, mail, enabled) VALUES ('telegram', '{"send":true,"receive":true}', 1)`,
		`INSERT INTO plugins(name, mail, enabled) VALUES ('paused', '{"send":true,"receive":true}', 0)`,
		`INSERT INTO plugins(name, mail, enabled) VALUES ('metrics', '{"send":true,"receive":false}', 1)`,
	} {
		if _, err := st.DB.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	d := New(st, "customer")
	for name, want := range map[string]bool{"telegram": true, "paused": false, "metrics": false, "ghost": false} {
		if got, err := d.PluginReceivesMail(name); err != nil || got != want {
			t.Fatalf("PluginReceivesMail(%s) = %v, %v; want %v", name, got, err, want)
		}
	}
	if ok, _ := d.PluginInstalled("paused"); !ok {
		t.Fatal("a disabled plugin is still installed")
	}
}

func TestDirectoryInstantSenders(t *testing.T) {
	st := storetest.Open(t)
	as := agent.NewStore(st)
	for _, a := range []agent.Agent{{Name: "lead", ImageRef: "i:latest", Group: "dev"},
		{Name: "dev1", ImageRef: "i:latest", Group: "dev"}, {Name: "solo", ImageRef: "i:latest"}} {
		if err := as.Create(a); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.DB.Exec(`INSERT INTO groups(name, lead) VALUES ('dev', 'lead')`); err != nil {
		t.Fatal(err)
	}
	d := New(st, "customer")
	if lead, err := d.GroupLead("dev1"); err != nil || lead != "lead" {
		t.Fatalf("lead of dev1 = %q, %v", lead, err)
	}
	if lead, err := d.GroupLead("solo"); err != nil || lead != "" {
		t.Fatalf("lead of solo = %q, %v; want none", lead, err)
	}
	if s, set, err := d.InstantSenders("dev1"); err != nil || set || s != nil {
		t.Fatalf("unset senders = %v, %v, %v", s, set, err)
	}
	if err := as.SetMailInstantSenders("dev1", []string{}); err != nil {
		t.Fatal(err)
	}
	if s, set, err := d.InstantSenders("dev1"); err != nil || !set || len(s) != 0 {
		t.Fatalf("empty senders = %v, %v, %v", s, set, err)
	}
	if err := as.SetMailInstantSenders("dev1", []string{"agent:solo"}); err != nil {
		t.Fatal(err)
	}
	if s, set, _ := d.InstantSenders("dev1"); !set || len(s) != 1 || s[0] != "agent:solo" {
		t.Fatalf("senders = %v, %v", s, set)
	}
	if err := as.SetMailInstantSenders("dev1", nil); err != nil {
		t.Fatal(err)
	}
	if _, set, _ := d.InstantSenders("dev1"); set {
		t.Fatal("reset senders are still set")
	}
	var _ mail.InstantDirectory = d
}
