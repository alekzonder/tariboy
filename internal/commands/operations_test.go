package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/operations"
	"github.com/alekzonder/tariboy/internal/registry"
)

func callCommand(t *testing.T, c *registry.Ctx, path string, params registry.Params) (any, error) {
	t.Helper()
	cmd, ok := BuildRegistry().Get(path)
	if !ok {
		t.Fatalf("command %s not registered", path)
	}
	return registry.Call(c, context.Background(), cmd, params)
}

// finishedOperation waits until operation id is no longer running.
func finishedOperation(t *testing.T, c *registry.Ctx, id int64) Operation {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		result, err := callCommand(t, c, "operation.show", registry.Params{"id": fmt.Sprint(id)})
		if err != nil {
			t.Fatal(err)
		}
		if op := result.(Operation); op.Status != operations.StatusRunning {
			return op
		}
		if time.Now().After(deadline) {
			t.Fatalf("operation %d is still running", id)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func operationLogText(t *testing.T, c *registry.Ctx, id int64) string {
	t.Helper()
	result, err := callCommand(t, c, "operation.log", registry.Params{"id": fmt.Sprint(id)})
	if err != nil {
		t.Fatal(err)
	}
	chunk := result.(OperationLog)
	if !chunk.Available || !chunk.Finished {
		t.Fatalf("log = %+v", chunk)
	}
	return chunk.Content
}

func TestImageBuildAnswersWithRunningOperation(t *testing.T) {
	c := localCtx(t)
	result, err := callCommand(t, c, "image.build", registry.Params{"name": "demo", "path": writeExample(t), "initiator": "ui"})
	if err != nil {
		t.Fatal(err)
	}
	started := result.(OperationStarted).Operation
	if started.Status != operations.StatusRunning || started.Kind != "image.build" || started.Initiator != "ui" || started.Title != "image build demo" || started.Params["name"] != "demo" {
		t.Fatalf("started = %+v", started)
	}
	op := finishedOperation(t, c, started.ID)
	if op.Status != operations.StatusSucceeded || op.ImageResult == nil || op.ImageResult.Digest == "" || op.FinishedAt == "" {
		t.Fatalf("finished = %+v", op)
	}
	log := operationLogText(t, c, started.ID)
	for _, want := range []string{"parse ", "freeze a snapshot", "build layers of demo:latest", "built demo:latest digest " + op.ImageResult.Digest, "succeeded in"} {
		if !strings.Contains(log, want) {
			t.Fatalf("log lacks %q:\n%s", want, log)
		}
	}
}

func TestImageBuildLogsTheFailingStep(t *testing.T) {
	c := localCtx(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Tariboyfile.yaml"), []byte("schema_version: 2\nprompts: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := callCommand(t, c, "image.build", registry.Params{"name": "broken", "path": dir, "wait": true})
	var ue api.UserError
	if !errors.As(err, &ue) || ue.Code != "bad_imagefile" {
		t.Fatalf("wait err = %v", err)
	}
	list, err := callCommand(t, c, "operation.ls", registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	op := list.(OperationList).Operations[0]
	if op.Status != operations.StatusFailed || op.ErrorCode != "bad_imagefile" || op.ErrorMessage != ue.Msg {
		t.Fatalf("operation = %+v", op)
	}
	log := operationLogText(t, c, op.ID)
	if !strings.Contains(log, "parse "+dir) || !strings.Contains(log, "failed: bad_imagefile: "+ue.Msg) {
		t.Fatalf("log:\n%s", log)
	}
}

func TestImageBuildRejectsBadArgumentsWithoutOperation(t *testing.T) {
	c := localCtx(t)
	_, err := callCommand(t, c, "image.build", registry.Params{"name": "demo"})
	var ue api.UserError
	if !errors.As(err, &ue) || ue.Code != "missing_path" || ue.Status != http.StatusBadRequest {
		t.Fatalf("err = %v", err)
	}
	_, err = callCommand(t, c, "image.build", registry.Params{"name": "demo", "path": filepath.Join(t.TempDir(), "missing")})
	if !errors.As(err, &ue) || ue.Code != "bad_source_path" || ue.Status != http.StatusBadRequest {
		t.Fatalf("missing path err = %v", err)
	}
	list, err := callCommand(t, c, "operation.ls", registry.Params{})
	if err != nil {
		t.Fatal(err)
	}
	if list.(OperationList).Total != 0 {
		t.Fatalf("list = %+v", list)
	}
}

func TestWorkflowBuildLogsValidationErrors(t *testing.T) {
	c := workflowCtx(t)
	dir := writeWorkflowSource(t, t.TempDir(), "1.0.0")
	if err := os.Remove(filepath.Join(dir, "statuses", "work.md")); err != nil {
		t.Fatal(err)
	}
	result, err := callCommand(t, c, "workflow.build", registry.Params{"path": dir})
	if err != nil {
		t.Fatal(err)
	}
	op := finishedOperation(t, c, result.(OperationStarted).Operation.ID)
	if op.Status != operations.StatusFailed || op.ErrorCode != "workflow_invalid" || op.Title != "workflow build "+dir {
		t.Fatalf("operation = %+v", op)
	}
	if log := operationLogText(t, c, op.ID); !strings.Contains(log, "invalid: ") || !strings.Contains(log, "work.md") {
		t.Fatalf("log:\n%s", log)
	}
}

func TestOperationListPagesNewestFirst(t *testing.T) {
	c := localCtx(t)
	for range 3 {
		if _, err := callCommand(t, c, "image.build", registry.Params{"name": "demo", "path": writeExample(t), "wait": true}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := callCommand(t, c, "operation.ls", registry.Params{"page": 2, "limit": 2})
	if err != nil {
		t.Fatal(err)
	}
	list := result.(OperationList)
	if list.Total != 3 || list.Page != 2 || len(list.Operations) != 1 || list.Operations[0].ID != 1 {
		t.Fatalf("list = %+v", list)
	}
	if _, err := callCommand(t, c, "operation.ls", registry.Params{"limit": 500}); err == nil {
		t.Fatal("limit 500 accepted")
	}
	if _, err := callCommand(t, c, "operation.show", registry.Params{"id": "99"}); err == nil {
		t.Fatal("missing operation found")
	}
}

func TestOperationLogReadsFromOffset(t *testing.T) {
	c := localCtx(t)
	if _, err := callCommand(t, c, "image.build", registry.Params{"name": "demo", "path": writeExample(t), "wait": true}); err != nil {
		t.Fatal(err)
	}
	full := operationLogText(t, c, 1)
	result, err := callCommand(t, c, "operation.log", registry.Params{"id": "1", "offset": 5})
	if err != nil {
		t.Fatal(err)
	}
	tail := result.(OperationLog)
	if tail.Offset != 5 || tail.Content != full[5:] || tail.Next != int64(len(full)) {
		t.Fatalf("tail = %+v", tail)
	}
	if err := os.Remove(c.Operations.Store().LogPath(1)); err != nil {
		t.Fatal(err)
	}
	result, err = callCommand(t, c, "operation.log", registry.Params{"id": "1"})
	if err != nil || result.(OperationLog).Available {
		t.Fatalf("removed log = %+v, %v", result, err)
	}
}

// fakeOperationCaller answers the CLI's build calls with a scripted operation.
type fakeOperationCaller struct {
	logs   []string
	reads  int
	final  Operation
	posted map[string]any
}

func (f *fakeOperationCaller) Call(method, route string, body any) (json.RawMessage, error) {
	switch {
	case method == http.MethodPost:
		f.posted = body.(map[string]any)
		return json.Marshal(OperationStarted{Operation: Operation{ID: 7, Title: "image build demo", Status: operations.StatusRunning}})
	case strings.HasSuffix(route, "/log"):
		chunk := OperationLog{ID: 7, Available: true, Offset: int64(f.reads)}
		if f.reads < len(f.logs) {
			chunk.Content = f.logs[f.reads]
		}
		f.reads++
		chunk.Next, chunk.Size, chunk.Finished = int64(f.reads), int64(f.reads), f.reads >= len(f.logs)
		return json.Marshal(chunk)
	default:
		return json.Marshal(f.final)
	}
}

func TestBuildLocalPrintsLogThenResult(t *testing.T) {
	layers := 2
	fake := &fakeOperationCaller{
		logs:  []string{"parse x\n", "succeeded\n"},
		final: Operation{ID: 7, Status: operations.StatusSucceeded, ImageResult: &ImageBuildResult{Name: "demo", Tag: "latest", Digest: "abc", Layers: &layers}},
	}
	var out, errOut bytes.Buffer
	c := &registry.Ctx{Out: &out, ErrOut: &errOut}
	if _, err := runBuildLocal(c, registry.Request{Ctx: context.Background(), Params: registry.Params{"name": "demo", "detach": false}}, fake, "/api/images/build"); err != nil {
		t.Fatal(err)
	}
	if fake.posted["initiator"] != "cli" || fake.posted["detach"] != nil {
		t.Fatalf("posted = %v", fake.posted)
	}
	if errOut.String() != "operation 7: image build demo\nparse x\nsucceeded\n" {
		t.Fatalf("stderr = %q", errOut.String())
	}
	if !strings.Contains(out.String(), "digest: abc\n") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestBuildLocalFailsWithOperationError(t *testing.T) {
	fake := &fakeOperationCaller{logs: []string{"failed\n"}, final: Operation{ID: 7, Status: operations.StatusFailed, ErrorCode: "bad_imagefile", ErrorMessage: "line 2"}}
	c := &registry.Ctx{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}}
	_, err := runBuildLocal(c, registry.Request{Ctx: context.Background(), Params: registry.Params{}}, fake, "/api/images/build")
	var ue api.UserError
	if !errors.As(err, &ue) || ue.Code != "bad_imagefile" || ue.Msg != "line 2" {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildLocalDetachPrintsOnlyTheID(t *testing.T) {
	fake := &fakeOperationCaller{}
	var out bytes.Buffer
	c := &registry.Ctx{Out: &out, ErrOut: &bytes.Buffer{}}
	if _, err := runBuildLocal(c, registry.Request{Ctx: context.Background(), Params: registry.Params{"detach": true}}, fake, "/api/images/build"); err != nil {
		t.Fatal(err)
	}
	if out.String() != "operation: 7\n" || fake.reads != 0 {
		t.Fatalf("stdout = %q, log reads %d", out.String(), fake.reads)
	}
}
