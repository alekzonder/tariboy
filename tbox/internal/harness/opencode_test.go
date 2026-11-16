package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

// openCodeHost answers `opencode debug config` run with OPENCODE_CONFIG_DIR
// in the extra environment with configOut, and `opencode --version` with
// 1.19.5. It has no env(1): the probe passes the variable through Host.Output.
func openCodeHost(configOut func(dir string) string) *fakeHost {
	return &fakeHost{
		installed: map[string]bool{"opencode": true},
		output: func(env []string, name string, args []string) ([]byte, error) {
			if name != "/fake/bin/opencode" {
				return nil, errors.New("unexpected command")
			}
			if slices.Equal(args, []string{"--version"}) && len(env) == 0 {
				return []byte("1.19.5\n"), nil
			}
			if slices.Equal(args, []string{"debug", "config"}) && len(env) == 1 {
				if dir, ok := strings.CutPrefix(env[0], "OPENCODE_CONFIG_DIR="); ok {
					return []byte(configOut(dir)), nil
				}
			}
			return nil, errors.New("unexpected command")
		},
	}
}

// echoConfig returns the probed directory's opencode.json, as the real
// `opencode debug config` would include its entries.
func echoConfig(dir string) string {
	body, _ := os.ReadFile(filepath.Join(dir, "opencode.json"))
	return string(body)
}

func TestOpenCodePrepare(t *testing.T) {
	run := twoSkillRun(t, "be helpful\n")
	f := openCodeHost(echoConfig)
	launch, err := openCode{}.Prepare(context.Background(), f.host(), run, []string{"--continue"})
	if err != nil {
		t.Fatal(err)
	}
	cfgDir := filepath.Join(run.Dir, "opencode")
	if launch.Path != "/fake/bin/opencode" || !slices.Equal(launch.Argv, []string{"opencode", "--continue"}) {
		t.Fatalf("launch %+v", launch)
	}
	wantEnv := []string{"OPENCODE_CONFIG_DIR=" + cfgDir, "TBOX_IMAGE_DIR=" + run.ImageDir, "TBOX_IMAGE=demo:latest"}
	if !slices.Equal(launch.Env, wantEnv) {
		t.Fatalf("env %v", launch.Env)
	}
	body, err := os.ReadFile(filepath.Join(cfgDir, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Skills struct {
			Paths []string `json:"paths"`
		} `json:"skills"`
		Instructions []string `json:"instructions"`
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Skills.Paths, []string{run.SkillsDir}) || !slices.Equal(cfg.Instructions, []string{run.PromptPath}) {
		t.Fatalf("opencode.json %s", body)
	}
	if len(launch.Notes) != 0 {
		t.Fatalf("notes %v", launch.Notes)
	}
}

func TestOpenCodeEmptyPromptOmitsInstructions(t *testing.T) {
	run := twoSkillRun(t, "")
	if _, err := (openCode{}).Prepare(context.Background(), openCodeHost(echoConfig).host(), run, nil); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(run.Dir, "opencode", "opencode.json"))
	if strings.Contains(string(body), "instructions") {
		t.Fatalf("opencode.json %s", body)
	}
}

func TestOpenCodeProbeWithoutPath(t *testing.T) {
	run := twoSkillRun(t, "p\n")
	f := openCodeHost(func(string) string { return `{"username":"x"}` })
	_, err := openCode{}.Prepare(context.Background(), f.host(), run, nil)
	var unsupported ErrUnsupportedVersion
	if !errors.As(err, &unsupported) || unsupported.Harness != "opencode" || unsupported.Found != "1.19.5" || unsupported.Minimum == "" {
		t.Fatalf("error %#v", err)
	}
}

func TestOpenCodeNotesReplacedConfigDir(t *testing.T) {
	run := twoSkillRun(t, "p\n")
	f := openCodeHost(echoConfig)
	f.env = map[string]string{"OPENCODE_CONFIG_DIR": "/home/u/my-opencode"}
	launch, err := openCode{}.Prepare(context.Background(), f.host(), run, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(launch.Notes) != 1 || !strings.Contains(launch.Notes[0], "/home/u/my-opencode") {
		t.Fatalf("notes %v", launch.Notes)
	}
}

func TestOpenCodeMissingBinary(t *testing.T) {
	_, err := openCode{}.Prepare(context.Background(), (&fakeHost{}).host(), twoSkillRun(t, ""), nil)
	if err == nil || !strings.Contains(err.Error(), "opencode") {
		t.Fatalf("error %v", err)
	}
}

func TestOpenCodeSimultaneousFirstLaunches(t *testing.T) {
	run := twoSkillRun(t, "p\n")
	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = openCode{}.Prepare(context.Background(), openCodeHost(echoConfig).host(), run, nil)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
}
