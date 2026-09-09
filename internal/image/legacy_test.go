package image

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

var legacyTestStore string

func TestMain(m *testing.M) {
	var err error
	legacyTestStore, err = os.MkdirTemp("", "tariboy-legacy-store-")
	if err != nil {
		panic(err)
	}
	files := map[string]string{
		"skills/whoami/SKILL.md":        "# Who you are\nscripts/whoami.sh\n",
		"skills/messages/SKILL.md":      "# Messages\nscripts/messages.sh\n",
		"skills/context/SKILL.md":       "# Context\nscripts/context.sh\n",
		"skills/status/SKILL.md":        "# Status\nscripts/status.sh\n",
		"skills/schedule/SKILL.md":      "# Schedule\nscripts/schedule.sh\n",
		"skills/scripts/SKILL.md":       "# Scripts\nscripts/scripts.sh\n",
		"skills/goal/SKILL.md":          "# Goal\nscripts/goal.sh\n",
		"skills/llm-as-judge/SKILL.md":  "# LLM as judge\nscripts/judge.sh\n",
		"skills/image-creator/SKILL.md": "# Image creator\nscripts/image_creator.sh\n",
		"skills/tasks/SKILL.md":         "# Tasks\nscripts/tasks.sh\n",
		"prompts/iteration-finish.md":   "Run i-am-done.\n",
	}
	for name, body := range files {
		path := filepath.Join(legacyTestStore, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			panic(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			panic(err)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(legacyTestStore)
	os.Exit(code)
}

func buildLegacy(t *testing.T, source *imagefile.Imagefile, ref Ref, store *Store, clock func() time.Time, options ...BuildOption) (Manifest, error) {
	t.Helper()
	return Build(source, ref, store, clock, append(options, WithBuiltinStoreRoot(legacyTestStore))...)
}

func buildMutableLegacy(t *testing.T, source *imagefile.Imagefile, ref Ref, store *Store, clock func() time.Time, options ...BuildOption) (Manifest, []byte, error) {
	t.Helper()
	return BuildMutableArchive(source, ref, store, clock, append(options, WithBuiltinStoreRoot(legacyTestStore))...)
}
