package agent

import "testing"

func TestSetScheduleRoundTripAndUpdateKeepsIt(t *testing.T) {
	as := newAliasStore(t)
	if err := as.Create(Agent{Name: "a1", OnTimeout: "restart", OnError: "restart"}); err != nil {
		t.Fatal(err)
	}
	if a, err := as.Get("a1"); err != nil || a.ScheduleRRule != "" {
		t.Fatalf("new agent schedule = %q, %v", a.ScheduleRRule, err)
	}
	rule := "DTSTART:20261018T140700\nRRULE:FREQ=DAILY;BYHOUR=9;BYMINUTE=30;BYSECOND=0"
	if err := as.SetSchedule("a1", rule); err != nil {
		t.Fatal(err)
	}
	a, err := as.Get("a1")
	if err != nil || a.ScheduleRRule != rule {
		t.Fatalf("schedule = %q, %v", a.ScheduleRRule, err)
	}
	// Update must NOT clobber the schedule (owned column).
	a.ScheduleRRule = ""
	if err := as.Update(a); err != nil {
		t.Fatal(err)
	}
	list, err := as.List()
	if err != nil || len(list) != 1 || list[0].ScheduleRRule != rule {
		t.Fatalf("list after update = %+v, %v", list, err)
	}
	if err := as.SetSchedule("missing", rule); err != ErrNotFound {
		t.Fatalf("missing agent err = %v, want ErrNotFound", err)
	}
}
