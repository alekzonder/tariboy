package compose

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alekzonder/tariboy/internal/client"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/workflowfile"
)

// sourceState is what a Store source gives at its current commit: the
// image_version of each image source and the workflow_version of each
// workflow source. err fails a create, ref change or refresh of a Store with
// this source.
type sourceState struct {
	commit    string
	images    map[string]string
	workflows map[string]string
	err       error
}

// fakeStore is a registered Store.
type fakeStore struct {
	source, ref, commit string
}

// sourcesCaller is a fake daemon with Stores, image builds that record
// image_version, and workflow builds from a Store or a path. The other routes
// go to workflowComposeCaller.
type sourcesCaller struct {
	*workflowComposeCaller
	remote     map[string]*sourceState // by Store source
	registered map[string]*fakeStore
	manifests  map[string]map[string]any // image NAME:TAG -> manifest
	// imageBuilds counts the image builds by name.
	imageBuilds map[string]int
}

func newSourcesCaller() *sourcesCaller {
	return &sourcesCaller{workflowComposeCaller: newWorkflowComposeCaller(), remote: map[string]*sourceState{},
		registered: map[string]*fakeStore{}, manifests: map[string]map[string]any{}, imageBuilds: map[string]int{}}
}

func (f *sourcesCaller) detail(name string) map[string]any {
	s := f.registered[name]
	st := f.remote[s.source]
	images, workflows := []map[string]any{}, []map[string]any{}
	for _, n := range sortedKeys(st.images) {
		images = append(images, map[string]any{"name": n, "version": st.images[n]})
	}
	for _, n := range sortedKeys(st.workflows) {
		workflows = append(workflows, map[string]any{"name": n, "version": st.workflows[n]})
	}
	return map[string]any{"name": name, "source": s.source, "ref": s.ref, "commit": s.commit, "images": images, "workflows": workflows}
}

func (f *sourcesCaller) Call(method, route string, body any) (json.RawMessage, error) {
	if raw, err, ok := f.storeCall(method, route, body); ok {
		return raw, err
	}
	switch {
	case method == "POST" && route == "/api/images/build":
		f.record(method, route, body)
		return f.buildImage(body.(map[string]any))
	case method == "GET" && strings.HasPrefix(route, "/api/images/"):
		f.record(method, route, body)
		m, ok := f.manifests[strings.TrimPrefix(route, "/api/images/")]
		if !ok {
			return nil, &client.APIError{Code: "not_found", Msg: "image not found"}
		}
		return mustJSON(m), nil
	case method == "POST" && route == "/api/workflow-images/build":
		f.record(method, route, body)
		return f.buildWorkflow(body.(map[string]any))
	case method == "POST" && route == "/api/workflow-images/validate":
		f.record(method, route, body)
		store, src, _ := strings.Cut(body.(map[string]any)["source"].(string), "/")
		return mustJSON(map[string]any{"valid": true, "name": src, "version": f.remote[f.registered[store].source].workflows[src]}), nil
	}
	return f.workflowComposeCaller.Call(method, route, body)
}

func (f *sourcesCaller) storeCall(method, route string, body any) (json.RawMessage, error, bool) {
	if !strings.HasPrefix(route, "/api/stores") {
		return nil, nil, false
	}
	f.record(method, route, body)
	parts := strings.Split(strings.TrimPrefix(route, "/api/stores"), "/")
	switch {
	case method == "GET" && route == "/api/stores":
		rows := []map[string]any{}
		for _, n := range sortedKeys(f.registered) {
			s := f.registered[n]
			rows = append(rows, map[string]any{"name": n, "source": s.source, "ref": s.ref, "commit": s.commit})
		}
		return mustJSON(rows), nil, true
	case method == "POST" && route == "/api/stores":
		b := body.(map[string]any)
		st := f.remote[b["source"].(string)]
		if st == nil {
			return nil, &client.APIError{Code: "bad_source_path", Msg: "no such source"}, true
		}
		if st.err != nil {
			return nil, st.err, true
		}
		name := b["name"].(string)
		f.registered[name] = &fakeStore{source: b["source"].(string), ref: b["ref"].(string), commit: st.commit}
		return mustJSON(map[string]any{"name": name}), nil, true
	}
	s := f.registered[parts[1]]
	if s == nil {
		return nil, &client.APIError{Code: "not_found", Msg: "store not found"}, true
	}
	st := f.remote[s.source]
	switch {
	case method == "GET" && len(parts) == 2:
		if st.err != nil {
			return nil, st.err, true
		}
		return mustJSON(f.detail(parts[1])), nil, true
	case method == "POST" && len(parts) == 3 && parts[2] == "ref":
		if st.err != nil {
			return nil, st.err, true
		}
		s.ref = body.(map[string]any)["ref"].(string)
		return mustJSON(f.detail(parts[1])), nil, true
	case method == "POST" && len(parts) == 3 && parts[2] == "refresh":
		if st.err != nil {
			return nil, st.err, true
		}
		s.commit = st.commit
		return mustJSON(f.detail(parts[1])), nil, true
	}
	return nil, errors.New("unexpected store call " + method + " " + route), true
}

// buildImage publishes NAME:TAG with the source's image_version; the digest
// follows the version, as a rebuild of the same content keeps its digest.
func (f *sourcesCaller) buildImage(b map[string]any) (json.RawMessage, error) {
	name, _ := b["name"].(string)
	tag, _ := b["tag"].(string)
	var version string
	if sel, _ := b["source"].(string); sel != "" {
		store, src, _ := strings.Cut(sel, "/")
		version = f.remote[f.registered[store].source].images[src]
	} else {
		v2, err := imagefile.ParseV2(b["path"].(string))
		if err != nil {
			return nil, err
		}
		version = v2.ImageVersion
	}
	f.imageBuilds[name]++
	m := map[string]any{"name": name, "tag": tag, "digest": "img-" + name + "-" + version, "image_version": version}
	f.manifests[name+":"+tag] = m
	return mustJSON(m), nil
}

// buildWorkflow publishes a workflow under its version and latest.
func (f *sourcesCaller) buildWorkflow(b map[string]any) (json.RawMessage, error) {
	var name, version string
	if sel, _ := b["source"].(string); sel != "" {
		if err := f.buildErr[sel]; err != nil {
			return nil, err
		}
		store, src, _ := strings.Cut(sel, "/")
		name, version = src, f.remote[f.registered[store].source].workflows[src]
	} else {
		wf, err := workflowfile.Parse(b["path"].(string))
		if err != nil {
			return nil, err
		}
		name, version = wf.Name, wf.WorkflowVersion
	}
	digest := "wf-" + name + "-" + version
	_, existed := f.images[name+":"+version]
	m := map[string]any{"name": name, "version": version, "digest": digest}
	f.images[name+":"+version], f.images[name+":latest"] = m, m
	return operationResult(map[string]any{"name": name, "version": version, "digest": digest, "created": !existed}), nil
}

// writeImageSource writes a Tariboyfile.yaml with image_version to dir.
func writeImageSource(t *testing.T, dir, version string) {
	t.Helper()
	writeSourceFile(t, filepath.Join(dir, "Tariboyfile.yaml"), "schema_version: 2\nimage_version: "+version+"\n")
}

// writeWorkflowSource writes a Workflowfile.yaml with name and version to dir.
func writeWorkflowSource(t *testing.T, dir, name, version string) {
	t.Helper()
	writeSourceFile(t, filepath.Join(dir, "Workflowfile.yaml"), "schema_version: 1\nname: "+name+"\nworkflow_version: "+version+"\n")
}

func writeSourceFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// reportOutcome returns the outcome of a resource in an up report.
func reportOutcome(out, resource string) string {
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.TrimSuffix(fields[1], ":") == resource {
			return fields[0]
		}
	}
	return ""
}
