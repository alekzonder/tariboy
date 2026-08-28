package telegramplugin

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStatePersistsNormalizedConfigurationAndTopics(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Configure("123:secret", []int64{22, 11, 22, -5}); err != nil {
		t.Fatal(err)
	}
	if err := store.BindChat(-100123, 7); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAgentTopic("agent-id", "worker", 9); err != nil {
		t.Fatal(err)
	}
	if err := store.SetOffset(42); err != nil {
		t.Fatal(err)
	}

	reloaded, err := OpenState(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Snapshot()
	if got.Token != "123:secret" || got.ChatID != -100123 || got.ManagementTopicID != 7 || got.Offset != 42 {
		t.Fatalf("state = %+v", got)
	}
	if !reflect.DeepEqual(got.AllowedUIDs, []int64{-5, 11, 22}) {
		t.Fatalf("allowed_uids = %#v", got.AllowedUIDs)
	}
	if got.AgentTopics["agent-id"].ThreadID != 9 || got.AgentTopics["agent-id"].Name != "worker" {
		t.Fatalf("agent topics = %#v", got.AgentTopics)
	}
	info, err := os.Stat(filepath.Join(dir, "telegram.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o", info.Mode().Perm())
	}
}

func TestConfigureWithoutTokenPreservesExistingTokenAndEmptyUIDsDenyAll(t *testing.T) {
	store, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Configure("token", []int64{1}); err != nil {
		t.Fatal(err)
	}
	if err := store.Configure("", nil); err != nil {
		t.Fatal(err)
	}
	got := store.Snapshot()
	if got.Token != "token" || len(got.AllowedUIDs) != 0 {
		t.Fatalf("state = %+v", got)
	}
}

func TestOpenStateRepairsExistingFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "telegram.json")
	if err := os.WriteFile(path, []byte(`{"bot_token":"secret","allowed_uids":[],"agent_topics":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenState(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o", info.Mode().Perm())
	}
}

func TestStateKeepsInMemoryConfigurationWhenAtomicSaveFails(t *testing.T) {
	store, err := OpenState(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Configure("working-token", []int64{11}); err != nil {
		t.Fatal(err)
	}
	store.path = filepath.Join(t.TempDir(), "missing", "telegram.json")
	if err := store.Configure("replacement-token", []int64{22}); err == nil {
		t.Fatal("configure succeeded with missing state directory")
	}
	got := store.Snapshot()
	if got.Token != "working-token" || len(got.AllowedUIDs) != 1 || got.AllowedUIDs[0] != 11 {
		t.Fatalf("failed save changed in-memory state: %+v", got)
	}
}
