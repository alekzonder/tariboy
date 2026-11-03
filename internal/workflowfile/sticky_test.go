package workflowfile

import (
	"encoding/json"
	"testing"
)

func TestParseStickyHolderOnBothLevels(t *testing.T) {
	f, err := Parse(writeManifest(t, `schema_version: 1
name: development
workflow_version: 0.11.0
initial_status: draft
sticky_holder: false
statuses:
  - id: draft
    owner: {pool: writers}
    sticky_holder: true
    transitions: [{on: done, to: implement}]
  - id: implement
    owner: {pool: developers}
    transitions: [{on: done, to: closed}]
  - id: closed
    terminal: true
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.StickyHolder {
		t.Fatalf("file sticky_holder = true, want false")
	}
	if f.Statuses[0].StickyHolder == nil || !*f.Statuses[0].StickyHolder {
		t.Fatalf("draft sticky_holder = %v, want true", f.Statuses[0].StickyHolder)
	}
	if f.Statuses[1].StickyHolder != nil {
		t.Fatalf("implement sticky_holder = %v, want unset", *f.Statuses[1].StickyHolder)
	}
	if !f.StatusStickyHolder("draft") || f.StatusStickyHolder("implement") {
		t.Fatalf("effective draft=%v implement=%v, want true false", f.StatusStickyHolder("draft"), f.StatusStickyHolder("implement"))
	}
}

func TestStatusStickyHolder(t *testing.T) {
	yes, no := true, false
	f := validFixture(t)
	if f.StatusStickyHolder("plan") {
		t.Fatal("default = true, want false")
	}
	f.StickyHolder = true
	if !f.StatusStickyHolder("plan") {
		t.Fatal("file true, status unset = false, want true")
	}
	f.Statuses[0].StickyHolder = &no
	if f.StatusStickyHolder("plan") {
		t.Fatal("file true, status false = true, want false")
	}
	f.StickyHolder = false
	f.Statuses[0].StickyHolder = &yes
	if !f.StatusStickyHolder("plan") {
		t.Fatal("file unset, status true = false, want true")
	}
	if f.StatusStickyHolder("nope") != f.StickyHolder {
		t.Fatal("unknown status must yield the file value")
	}
}

func TestValidateStickyHolderOnlyOnPoolStatus(t *testing.T) {
	yes, no := true, false
	f := validFixture(t)
	f.StickyHolder = true
	f.Statuses[0].StickyHolder = &no
	if errs := Validate(f); len(errs) != 0 {
		t.Fatalf("pool status: errors = %+v", errs)
	}
	for i, val := range map[int]*bool{1: &yes, 2: &no, 3: &yes} {
		f := validFixture(t)
		f.Statuses[i].StickyHolder = val
		errs := Validate(f)
		path := "statuses[" + string(rune('0'+i)) + "].sticky_holder"
		if !hasError(errs, "sticky_holder_not_pool", path) {
			t.Fatalf("status %d: errors = %+v, want sticky_holder_not_pool at %s", i, errs, path)
		}
	}
}

func TestManifestWithoutStickyHolderReadsFalse(t *testing.T) {
	var f File
	data := `{"schema_version":1,"name":"development","workflow_version":"0.1.0","initial_status":"plan",
"statuses":[{"id":"plan","owner":{"kind":"pool","pool":"developers"}}]}`
	if err := json.Unmarshal([]byte(data), &f); err != nil {
		t.Fatal(err)
	}
	if f.StickyHolder || f.Statuses[0].StickyHolder != nil || f.StatusStickyHolder("plan") {
		t.Fatalf("old manifest: file=%v status=%v", f.StickyHolder, f.Statuses[0].StickyHolder)
	}
	f.StickyHolder = true
	out, err := json.Marshal(&f)
	if err != nil {
		t.Fatal(err)
	}
	var back File
	if err := json.Unmarshal(out, &back); err != nil || !back.StatusStickyHolder("plan") {
		t.Fatalf("round trip: err=%v value=%v", err, back.StickyHolder)
	}
}
