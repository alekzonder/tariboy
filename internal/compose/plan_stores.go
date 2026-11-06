package compose

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/alekzonder/tariboy/internal/client"
)

// storesPhase registers each Store of the file, sets the ref of a git Store
// and refreshes it. A path Store is only read: a refresh of a local Store
// that is a git checkout runs git pull in the user's directory. Every action
// of a Store is named "store:NAME", so a failed Store skips the builds from
// it.
type storesPhase struct{}

func (storesPhase) name() string { return "stores" }

func (storesPhase) plan(p *planner, f File, live *liveState) error {
	for _, name := range sortedKeys(f.Stores) {
		p.planStore(name, f.Stores[name], live.sources)
	}
	return nil
}

func (p *planner) planStore(name string, want StoreSpec, live liveSources) {
	id := "store:" + name
	source := p.r.storeSource(want)
	cur, ok := live.stores[name]
	switch {
	case !ok:
		p.status(id, true, "store      %-10s MISSING", name)
		p.provide(id, p.add(Action{Resource: id, Kind: "create", Note: "creating store " + name, Fail: "create store " + name,
			Fields: []Field{{Name: "source", To: source}, {Name: "ref", To: refText(want.Ref)}},
			Call:   Call{"POST", "/api/stores", map[string]any{"name": name, "source": source, "ref": want.Ref}}}))
		return
	case cur.Source != source:
		p.status(id, true, "store      %-10s source differs: have=%s want=%s (up does not change it)", name, cur.Source, source)
		err := fmt.Errorf("store %s is registered with source %s; the file declares %s; compose does not change a Store source (remove it with tariboy store rm %s, then run up)",
			name, cur.Source, source, name)
		p.provide(id, p.add(Action{Resource: id, Kind: "conflict", Fields: []Field{{Name: "source", From: cur.Source, To: source}},
			run: func([]json.RawMessage) (json.RawMessage, bool, error) { return nil, false, err }}))
		return
	}
	switch err := live.detailErr[name]; {
	case err != nil:
		p.status(id, true, "store      %-10s UNREADABLE (%s)", name, apiCode(err))
	case want.Path != "":
		p.status(id, false, "store      %-10s ok (path %s)", name, source)
	case cur.Ref != want.Ref:
		p.status(id, true, "store      %-10s drift: ref have=%s want=%s", name, refText(cur.Ref), refText(want.Ref))
	default:
		p.status(id, false, "store      %-10s ok (ref %s, commit %s)", name, refText(cur.Ref), shortDigest(cur.Commit))
	}
	if want.Path != "" {
		p.provide(id, p.add(Action{Resource: id, Kind: storeCheck, run: p.r.checkStore(name)}))
		return
	}
	ref := -1
	if cur.Ref != want.Ref {
		ref = p.add(Action{Resource: id, Kind: "ref", Note: "setting store " + name + " ref", Fail: "set store " + name + " ref",
			Fields: []Field{{Name: "ref", From: refText(cur.Ref), To: refText(want.Ref)}},
			Call:   Call{"POST", "/api/stores/" + name + "/ref", map[string]any{"ref": want.Ref}}})
		p.provide(id, ref)
	}
	p.provide(id, p.add(Action{Resource: id, Kind: "refresh", run: p.r.refreshStore(name, cur.Commit)}, ref))
}

// storeCheck is the kind of the read that checks a path Store; it changes
// nothing, so dry-run does not list it.
const storeCheck = "check"

func refText(ref string) string {
	if ref == "" {
		return "(default branch)"
	}
	return ref
}

// apiCode is the daemon's error code, or the error text without one.
func apiCode(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.Code != "" {
		return apiErr.Code
	}
	return err.Error()
}

// refreshStore refreshes a git Store; it changed when the commit moved.
func (r *Runner) refreshStore(name, commit string) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		r.logf("refreshing store %s", name)
		raw, err := r.call.Call("POST", "/api/stores/"+name+"/refresh", map[string]any{})
		if err != nil {
			return nil, false, fmt.Errorf("refresh store %s: %w", name, err)
		}
		var d storeDetail
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, false, fmt.Errorf("refresh store %s: %w", name, err)
		}
		return raw, d.Commit != commit, nil
	}
}

// checkStore reads a path Store; the daemon reads its directory again.
func (r *Runner) checkStore(name string) func([]json.RawMessage) (json.RawMessage, bool, error) {
	return func([]json.RawMessage) (json.RawMessage, bool, error) {
		raw, err := r.call.Call("GET", "/api/stores/"+name, map[string]string{})
		if err != nil {
			return nil, false, fmt.Errorf("read store %s: %w", name, err)
		}
		return raw, false, nil
	}
}
