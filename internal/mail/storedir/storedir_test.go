package storedir

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
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
