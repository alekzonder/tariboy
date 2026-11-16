package build

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
	"github.com/alekzonder/tariboy/tbox/internal/tfile"
)

const maxPromptFile = 4 << 20

// Options configures one build.
type Options struct {
	SourceDir string
	Name      string
	Tags      []string
	Store     imgstore.Store
	Now       func() time.Time
}

// Result describes the image a build produced or found up to date.
type Result struct {
	Name         string
	Digest       string
	ImageVersion string
	Tags         []string
	Built        bool
	Moved        []string
}

func writeAtomic(path string, data []byte) error {
	return imgstore.WriteFileAtomic(path, data)
}

// Build builds the image of opts.SourceDir into the store unless the source
// fingerprint equals the one of the last build and its image is still stored
// under every requested tag.
func Build(opts Options) (Result, error) {
	dir, err := filepath.Abs(opts.SourceDir)
	if err != nil {
		return Result{}, err
	}
	name := opts.Name
	if name == "" {
		name = filepath.Base(dir)
	}
	if err := imgstore.CheckName(name); err != nil {
		return Result{}, err
	}
	loaded, err := tfile.Load(dir)
	if err != nil {
		return Result{}, err
	}
	f := loaded.ForTbox()
	tags, err := buildTags(opts.Tags, f.ImageVersion)
	if err != nil {
		return Result{}, err
	}
	cache := loadCache(opts.Store.StatCachePath(name))
	fp, err := fingerprint(f, cache)
	if err != nil {
		return Result{}, err
	}
	if err := cache.save(opts.Store.StatCachePath(name)); err != nil {
		return Result{}, err
	}
	res := Result{Name: name, ImageVersion: f.ImageVersion, Tags: tags}
	if digest, ok := upToDate(opts.Store, name, fp, tags); ok {
		res.Digest = digest
		return res, nil
	}
	img, err := assemble(f, name, dir, opts.now())
	if err != nil {
		return Result{}, err
	}
	if res.Moved, err = opts.Store.Publish(name, img, tags); err != nil {
		return Result{}, err
	}
	if err := opts.Store.SetFingerprint(name, fp, img.Manifest.Digest); err != nil {
		return Result{}, err
	}
	res.Digest, res.Built = img.Manifest.Digest, true
	return res, nil
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func upToDate(store imgstore.Store, name, fp string, tags []string) (string, bool) {
	oldFP, digest, ok := store.Fingerprint(name)
	if !ok || oldFP != fp {
		return "", false
	}
	for _, tag := range tags {
		got, _, err := store.Resolve(imgstore.Ref{Name: name, Tag: tag})
		if err != nil || got != digest {
			return "", false
		}
	}
	return digest, true
}

// buildTags returns the tags to publish: the requested ones, or image_version
// and latest. Duplicates are dropped.
func buildTags(requested []string, imageVersion string) ([]string, error) {
	tags := requested
	if len(tags) == 0 {
		if imageVersion != "" {
			tags = append(tags, imageVersion)
		}
		tags = append(tags, "latest")
	}
	seen := map[string]bool{}
	var out []string
	for _, tag := range tags {
		if err := imgstore.CheckTag(tag); err != nil {
			return nil, err
		}
		if !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out, nil
}

func assemble(f *tfile.File, name, dir string, builtAt time.Time) (archive.Image, error) {
	skills := make([]skill.Prepared, 0, len(f.Skills))
	for _, entry := range f.Skills {
		abs, source, category, err := tfile.ResolveSkillDir(entry)
		if err != nil {
			return archive.Image{}, err
		}
		prepared, err := skill.Prepare(abs, source, category)
		if err != nil {
			return archive.Image{}, fmt.Errorf("skill %s: %w", entry.Dir, err)
		}
		skills = append(skills, prepared)
	}
	prompts, err := promptLayers(f)
	if err != nil {
		return archive.Image{}, err
	}
	role, err := tfile.RoleRead(dir)
	if err != nil {
		return archive.Image{}, err
	}
	return archive.Assemble(name, f.ImageVersion, builtAt, skills, prompts, role)
}

func promptLayers(f *tfile.File) ([]archive.PromptLayer, error) {
	var layers []archive.PromptLayer
	skillDirs := f.SkillDirs()
	for _, entry := range f.Prompts {
		abs, source, category, err := tfile.ResolvePromptFile(entry, skillDirs...)
		if err != nil {
			return nil, err
		}
		body, err := fsafe.ReadFileNoFollow(abs, maxPromptFile)
		if err != nil {
			return nil, fmt.Errorf("prompt %s: %w", entry.File, err)
		}
		layers = append(layers, archive.PromptLayer{Source: source, Category: category, Base: filepath.Base(abs), Body: body})
	}
	return layers, nil
}
