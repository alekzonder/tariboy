package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/alekzonder/tariboy/tbox/internal/fetch"
	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
	"gopkg.in/yaml.v3"
)

const metaName = "meta.json"

// treeMeta sits next to the extracted skill so a pinned entry finds its
// directory without the archive body hash.
type treeMeta struct {
	Name       string `json:"name"`
	TreeSHA256 string `json:"tree_sha256"`
}

func (r Resolver) urlPrefix(u string) string { return filepath.Join(r.sourcesDir(), hexSum([]byte(u))) }

// findURLTree returns the cached skill directory of u whose tree hash is sha.
func (r Resolver) findURLTree(u, sha string) (string, bool) {
	matches, _ := filepath.Glob(r.urlPrefix(u) + "-*")
	for _, dir := range matches {
		data, err := os.ReadFile(filepath.Join(dir, metaName))
		if err != nil {
			continue
		}
		var m treeMeta
		if json.Unmarshal(data, &m) == nil && m.TreeSHA256 == sha && m.Name != "" && exists(filepath.Join(dir, m.Name)) {
			return filepath.Join(dir, m.Name), true
		}
	}
	return "", false
}

func (r Resolver) urlSkill(ctx context.Context, e tfile.SkillEntry, pinned *lock.Entry) (string, lock.Entry, error) {
	if pinned != nil {
		if dir, ok := r.findURLTree(e.URL, pinned.SHA256); ok {
			return dir, *pinned, nil
		}
	}
	unlock, err := r.lockSource(e.URL)
	if err != nil {
		return "", lock.Entry{}, err
	}
	defer unlock()
	body, err := r.HTTP.Get(ctx, e.URL, fetch.DefaultLimits.MaxCompressed)
	if err != nil {
		return "", lock.Entry{}, err
	}
	pinnedSHA := ""
	if pinned != nil {
		pinnedSHA = pinned.SHA256
	}
	got := lock.Entry{Kind: "url", URL: e.URL}
	dir, sha, err := r.storeArchive(body, e.URL, pinnedSHA)
	if err != nil {
		return "", lock.Entry{}, err
	}
	got.SHA256 = sha
	return dir, got, nil
}

// storeArchive extracts body into the cache entry of its hash and returns the
// skill directory and its tree hash.
func (r Resolver) storeArchive(body []byte, u, pinnedSHA string) (string, string, error) {
	final := r.urlPrefix(u) + "-" + hexSum(body)[:16]
	if m, ok := readMeta(final); ok {
		if pinnedSHA != "" && m.TreeSHA256 != pinnedSHA {
			return "", "", fmt.Errorf("remote content changed since it was locked; %s", updateHint)
		}
		return filepath.Join(final, m.Name), m.TreeSHA256, nil
	}
	stage, err := r.newStage()
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(stage)
	tmp := filepath.Join(stage, "extract")
	if err := fetch.ExtractArchive(body, tmp); err != nil {
		return "", "", err
	}
	name, err := skillName(tmp)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(stage, name)
	if err := os.Rename(tmp, dir); err != nil {
		return "", "", err
	}
	sha, err := treeSHA(dir, "url:"+u, pinnedSHA)
	if err != nil {
		return "", "", err
	}
	meta, _ := json.Marshal(treeMeta{Name: name, TreeSHA256: sha})
	if err := os.WriteFile(filepath.Join(stage, metaName), meta, 0o644); err != nil {
		return "", "", err
	}
	if err := publish(stage, final); err != nil {
		return "", "", err
	}
	return filepath.Join(final, name), sha, nil
}

func readMeta(dir string) (treeMeta, bool) {
	var m treeMeta
	data, err := os.ReadFile(filepath.Join(dir, metaName))
	if err != nil || json.Unmarshal(data, &m) != nil || m.Name == "" {
		return treeMeta{}, false
	}
	return m, true
}

// skillName reads the name from the SKILL.md front matter in dir. The skill
// package re-validates it against the directory name.
func skillName(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return "", errors.New("archive has no SKILL.md at its root")
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return "", errors.New("SKILL.md has no front matter")
	}
	front, _, ok := strings.Cut(rest, "\n---")
	if !ok {
		return "", errors.New("SKILL.md front matter is not closed")
	}
	var fm struct {
		Name string `yaml:"name"`
	}
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil || fm.Name == "" {
		return "", errors.New("SKILL.md front matter has no name")
	}
	if !filepath.IsLocal(fm.Name) || strings.ContainsAny(fm.Name, `/\`) {
		return "", fmt.Errorf("invalid skill name %q", fm.Name)
	}
	return fm.Name, nil
}

func (r Resolver) urlPrompt(ctx context.Context, e tfile.PromptEntry, pinned *lock.Entry) (string, lock.Entry, error) {
	name := urlFileName(e.URL)
	if pinned != nil && len(pinned.SHA256) >= 16 {
		if file := filepath.Join(r.urlPrefix(e.URL)+"-"+pinned.SHA256[:16], name); exists(file) {
			return file, *pinned, nil
		}
	}
	unlock, err := r.lockSource(e.URL)
	if err != nil {
		return "", lock.Entry{}, err
	}
	defer unlock()
	body, err := r.HTTP.Get(ctx, e.URL, maxPromptBytes)
	if err != nil {
		return "", lock.Entry{}, err
	}
	sha := hexSum(body)
	if pinned != nil && pinned.SHA256 != "" && pinned.SHA256 != sha {
		return "", lock.Entry{}, fmt.Errorf("remote content changed since it was locked; %s", updateHint)
	}
	file, err := r.storeFile(r.urlPrefix(e.URL)+"-"+sha[:16], name, body)
	return file, lock.Entry{Kind: "url", URL: e.URL, SHA256: sha}, err
}

// urlFileName is the last path element of u, or prompt.md.
func urlFileName(u string) string {
	if parsed, err := url.Parse(u); err == nil {
		base := path.Base(parsed.Path)
		if base != "." && base != "/" && base != ".." && filepath.IsLocal(base) {
			return base
		}
	}
	return "prompt.md"
}
