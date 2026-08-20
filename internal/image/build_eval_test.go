package image

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

func TestBuildInlinesEvalPromptContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "task.md"), []byte("do the task"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "judge.md"), []byte("Did the agent follow the task? Answer PASS or FAIL."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Tariboyfile.yaml"), []byte(`schema_version: 1
prompts:
  - task.md
evals:
  - { name: followed-task, type: llm-judge, prompt: judge.md }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	imgFile, err := imagefile.Parse(filepath.Join(dir, "Tariboyfile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	st := &Store{Dir: t.TempDir()}
	ref, _ := ParseRef("evaldemo:latest")
	if _, err := Build(imgFile, ref, st, func() time.Time { return time.Unix(0, 0).UTC() }); err != nil {
		t.Fatal(err)
	}
	man, err := st.Inspect(ref)
	if err != nil {
		t.Fatal(err)
	}
	if len(man.Evals) != 1 {
		t.Fatalf("evals = %+v", man.Evals)
	}
	if man.Evals[0].Prompt != "Did the agent follow the task? Answer PASS or FAIL." {
		t.Fatalf("prompt not inlined as content: %q", man.Evals[0].Prompt)
	}
}
