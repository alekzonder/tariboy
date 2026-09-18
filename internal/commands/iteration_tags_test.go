package commands

import (
	"testing"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

// taggedCtx seeds one agent with three iterations on three consecutive days.
func taggedCtx(t *testing.T) (*registry.Ctx, *agent.Store) {
	t.Helper()
	c, as, _ := ctxWithStore(t)
	as.Create(agent.Agent{Name: "smoke", OnTimeout: "restart", OnError: "restart"})
	as.Create(agent.Agent{Name: "other", OnTimeout: "restart", OnError: "restart"})
	for id, started := range map[string]string{
		"it-1": "2026-09-10T02:00:00Z",
		"it-2": "2026-09-11T02:00:00Z",
		"it-3": "2026-09-12T02:00:00Z",
	} {
		as.CreateIteration(agent.Iteration{ID: id, Agent: "smoke", Trigger: "interval",
			Status: "done", StartedAt: started})
	}
	as.CreateIteration(agent.Iteration{ID: "foreign", Agent: "other", Trigger: "interval",
		Status: "done", StartedAt: "2026-09-11T02:00:00Z"})
	return c, as
}

func lsIDs(t *testing.T, c *registry.Ctx, p registry.Params) []string {
	t.Helper()
	res, err := h(t, "iteration.ls")(c, p)
	if err != nil {
		t.Fatalf("iteration.ls %v: %v", p, err)
	}
	var out []string
	for _, row := range res.(map[string]any)["iterations"].([]map[string]any) {
		out = append(out, row["id"].(string))
	}
	return out
}

func TestIterationTagCommandsAddRemoveSetInBatch(t *testing.T) {
	c, _ := taggedCtx(t)

	res, err := h(t, "iteration.tag.add")(c, registry.Params{
		"name": "smoke", "id": []string{"it-1", "it-2"}, "tag": []string{"processed", "review"}})
	if err != nil {
		t.Fatal(err)
	}
	tags := res.(map[string]any)["tags"].(map[string][]string)
	if len(tags["it-1"]) != 2 || len(tags["it-2"]) != 2 {
		t.Fatalf("add tags = %v", tags)
	}

	if _, err := h(t, "iteration.tag.rm")(c, registry.Params{
		"name": "smoke", "id": []string{"it-1"}, "tag": []string{"review"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := h(t, "iteration.tag.set")(c, registry.Params{
		"name": "smoke", "id": []string{"it-2"}, "tag": []string{"done"}}); err != nil {
		t.Fatal(err)
	}

	res, err = h(t, "iteration.inspect")(c, registry.Params{"name": "smoke", "id": "it-2"})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.(map[string]any)["tags"].([]string); len(got) != 1 || got[0] != "done" {
		t.Fatalf("inspect tags = %v", got)
	}
}

func TestIterationTagCommandsRejectBadInput(t *testing.T) {
	c, _ := taggedCtx(t)
	cases := map[string]registry.Params{
		"invalid_tag": {"name": "smoke", "id": []string{"it-1"}, "tag": []string{"two words"}},
		"not_found":   {"name": "smoke", "id": []string{"it-1", "foreign"}, "tag": []string{"processed"}},
	}
	for wantCode, params := range cases {
		_, err := h(t, "iteration.tag.add")(c, params)
		ue, ok := err.(api.UserError)
		if !ok || ue.Code != wantCode {
			t.Fatalf("%s: err = %v", wantCode, err)
		}
	}
	// A foreign id must not have written the sibling id's tag either.
	if got := lsIDs(t, c, registry.Params{"name": "smoke", "tag": "processed"}); len(got) != 0 {
		t.Fatalf("partial write leaked: %v", got)
	}
}

func TestIterationLsFiltersByTagAndStartDate(t *testing.T) {
	c, _ := taggedCtx(t)
	if _, err := h(t, "iteration.tag.add")(c, registry.Params{
		"name": "smoke", "id": []string{"it-1"}, "tag": []string{"processed"}}); err != nil {
		t.Fatal(err)
	}

	if got := lsIDs(t, c, registry.Params{"name": "smoke"}); len(got) != 3 {
		t.Fatalf("unfiltered = %v", got)
	}
	// Query-string form: one comma-separated value, matching ANY listed tag.
	if got := lsIDs(t, c, registry.Params{"name": "smoke", "tag": "processed,absent"}); len(got) != 1 || got[0] != "it-1" {
		t.Fatalf("by tag = %v", got)
	}
	if got := lsIDs(t, c, registry.Params{"name": "smoke", "started-after": "2026-09-11"}); len(got) != 2 {
		t.Fatalf("started-after = %v", got)
	}
	if got := lsIDs(t, c, registry.Params{"name": "smoke", "started-before": "2026-09-10"}); len(got) != 1 || got[0] != "it-1" {
		t.Fatalf("started-before = %v", got)
	}

	// Rows carry their tags so a list view needs no second request.
	res, err := h(t, "iteration.ls")(c, registry.Params{"name": "smoke", "tag": "processed"})
	if err != nil {
		t.Fatal(err)
	}
	row := res.(map[string]any)["iterations"].([]map[string]any)[0]
	if got := row["tags"].([]string); len(got) != 1 || got[0] != "processed" {
		t.Fatalf("ls row tags = %v", got)
	}

	_, err = h(t, "iteration.ls")(c, registry.Params{"name": "smoke", "started-after": "yesterday"})
	ue, ok := err.(api.UserError)
	if !ok || ue.Code != "invalid_time" {
		t.Fatalf("bad date = %v", err)
	}
}
