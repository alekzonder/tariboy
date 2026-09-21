package commands

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/image"
	"github.com/alekzonder/tariboy/internal/imagefile"
	"github.com/alekzonder/tariboy/internal/registry"
)

type imageStatusJSON struct {
	Current, Next struct {
		Ref, Digest, Error, Reason string
		ImageVersion               string `json:"image_version"`
	}
	Pending agent.ImageAssignment
}

func readAgentImageStatus(t *testing.T, c *registry.Ctx) imageStatusJSON {
	t.Helper()
	w := httptest.NewRecorder()
	api.NewServer(BuildRegistry(), c).Handler().ServeHTTP(w, httptest.NewRequest("GET", "/api/agents/worker/image", nil))
	var body struct{ Result imageStatusJSON }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil {
		t.Fatalf("image status: %d %s", w.Code, w.Body.String())
	}
	return body.Result
}

func TestAgentImageProjection(t *testing.T) {
	for _, scenario := range []string{"rebuilt", "same version", "unchanged", "pending", "unversioned", "broken latest", "broken current", "broken pending", "activation error"} {
		t.Run(scenario, func(t *testing.T) {
			c := localCtx(t)
			as := agentStore(c)
			ref := image.Ref{Name: "reviewer", Tag: "latest"}
			src := t.TempDir()
			build := func(version, body string) image.Manifest {
				t.Helper()
				if err := os.WriteFile(filepath.Join(src, "prompt.md"), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				spec := &imagefile.V2{SchemaVersion: 2, ImageVersion: version, Dir: src, Prompts: []imagefile.PromptEntry{{File: "./prompt.md"}}}
				manifest, err := image.BuildV2(spec, imagefile.ResolveRoots{}, ref, imageStore(c), time.Now, nil)
				if err != nil {
					t.Fatal(err)
				}
				return manifest
			}
			version := "1.0.0"
			if scenario == "unversioned" {
				version = ""
			}
			first := build(version, "first")
			a := agent.Agent{Name: "worker", ImageRef: ref.String(), ImageDigest: first.Digest}
			if err := as.Create(a); err != nil {
				t.Fatal(err)
			}
			nextVersion, nextDigest, reason := version, first.Digest, "ref_moved"
			if scenario != "unchanged" && scenario != "unversioned" {
				nextVersion = "1.1.0"
				if scenario == "same version" {
					nextVersion = version
				}
				nextDigest = build(nextVersion, "second").Digest
			}
			if scenario == "pending" || scenario == "broken pending" {
				if err := as.SetPendingImage("worker", ref.String(), first.Digest); err != nil {
					t.Fatal(err)
				}
				nextDigest, nextVersion, reason = first.Digest, version, "pending"
			}
			if scenario == "broken latest" {
				// Corrupt only the content latest points at; the pinned
				// generation the agent still runs stays intact.
				pointer, err := os.ReadFile(filepath.Join(c.BaseDir, "images", "reviewer", "tags", "latest"))
				if err != nil {
					t.Fatal(err)
				}
				content := filepath.Join(c.BaseDir, "images", "reviewer", "refs", strings.TrimSpace(string(pointer))+".tar.gz")
				if err := os.WriteFile(content, []byte("broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "broken current" {
				a.ImageDigest = "missing"
				if _, err := c.Store.DB.Exec(`UPDATE agents SET image_digest=? WHERE name='worker'`, a.ImageDigest); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "broken pending" {
				if err := as.SetPendingImage("worker", ref.String(), "missing"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "activation error" {
				if _, err := as.SetPendingImageErrorIfEmpty("worker", "bridge preparation failed"); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := as.Get("worker")
			pendingBefore, _ := as.PendingImage("worker")
			got := readAgentImageStatus(t, c)
			if got.Current.Ref != ref.String() || got.Current.Digest != a.ImageDigest {
				t.Errorf("current identity: %+v", got.Current)
			}
			if scenario == "broken current" {
				if got.Current.Error == "" || got.Current.ImageVersion != "" {
					t.Errorf("missing pinned version hidden: %+v", got.Current)
				}
			} else if got.Current.ImageVersion != version || got.Current.Error != "" {
				t.Errorf("current version moved: %+v", got.Current)
			}
			if got.Next.Reason != reason || got.Next.Ref != ref.String() {
				t.Errorf("next selection: %+v", got.Next)
			}
			if scenario == "broken latest" || scenario == "broken pending" {
				if got.Next.Error == "" || got.Next.ImageVersion != "" {
					t.Errorf("next inspection error hidden: %+v", got.Next)
				}
			} else if got.Next.Digest != nextDigest || got.Next.ImageVersion != nextVersion || got.Next.Error != "" {
				t.Errorf("next image: %+v, want %s %s", got.Next, nextVersion, nextDigest)
			}
			if scenario == "activation error" && got.Pending.Error != "bridge preparation failed" {
				t.Errorf("activation error hidden: %+v", got)
			}
			after, _ := as.Get("worker")
			pendingAfter, _ := as.PendingImage("worker")
			if !reflect.DeepEqual(before, after) || pendingBefore != pendingAfter {
				t.Fatal("GET mutated image assignment")
			}
		})
	}
}

func TestAgentImageReadWaitsForPublicationGate(t *testing.T) {
	for _, command := range []string{"agent.image.status", "agent.image.cancel"} {
		t.Run(command, func(t *testing.T) {
			c := localCtx(t)
			ref := image.Ref{Name: "reviewer", Tag: "latest"}
			spec := &imagefile.V2{SchemaVersion: 2, ImageVersion: "1.0.0"}
			first, err := image.BuildV2(spec, imagefile.ResolveRoots{}, ref, imageStore(c), time.Now, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := agentStore(c).Create(agent.Agent{Name: "worker", ImageRef: ref.String(), ImageDigest: first.Digest}); err != nil {
				t.Fatal(err)
			}
			published, release, finished := make(chan error, 1), make(chan struct{}), make(chan error, 1)
			go func() {
				finished <- image.WithPublicationGate(func() error {
					spec.ImageVersion = "9.0.0"
					_, err := image.BuildV2(spec, imagefile.ResolveRoots{}, ref, imageStore(c), time.Now, nil)
					published <- err
					<-release
					if err != nil {
						return err
					}
					return imageStore(c).SetTag(ref, first.Digest)
				})
			}()
			if err := <-published; err != nil {
				close(release)
				<-finished
				t.Fatal(err)
			}
			type response struct {
				value any
				err   error
			}
			read := make(chan response, 1)
			handler := cmdHandler(t, command)
			go func() { value, err := handler(c, registry.Params{"name": "worker"}); read <- response{value, err} }()
			var result response
			early := false
			select {
			case result = <-read:
				early = true
			case <-time.After(100 * time.Millisecond):
			}
			close(release)
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			if !early {
				select {
				case result = <-read:
				case <-time.After(2 * time.Second):
					t.Fatal("image read did not release publication gate")
				}
			}
			if early || result.err != nil {
				t.Fatalf("read crossed publication: early=%v err=%v", early, result.err)
			}
			data, err := json.Marshal(result.value)
			var got imageStatusJSON
			if err != nil || json.Unmarshal(data, &got) != nil || got.Next.Digest != first.Digest || got.Next.ImageVersion != "1.0.0" {
				t.Fatalf("read uncommitted generation: %s err=%v", data, err)
			}
		})
	}
}
