package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/alekzonder/tariboy/tbox/internal/archive"
	"github.com/alekzonder/tariboy/tbox/internal/flock"
	"github.com/alekzonder/tariboy/tbox/internal/fsafe"
	"github.com/alekzonder/tariboy/tbox/internal/imgstore"
	"github.com/alekzonder/tariboy/tbox/internal/lock"
	"github.com/alekzonder/tariboy/tbox/internal/skill"
	srcpkg "github.com/alekzonder/tariboy/tbox/internal/source"
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
	// Update re-resolves every remote entry and rewrites the lock file.
	Update bool
	// Resolver fetches remote entries; nil makes a remote entry an error.
	Resolver *srcpkg.Resolver
	// StoreRoot is the root of the Store checkout that holds SourceDir, or
	// empty. Its skills-lock.json applies to a schema v2 source.
	StoreRoot string
}

// Result describes the image a build produced or found up to date.
type Result struct {
	Name         string
	Digest       string
	ImageVersion string
	Tags         []string
	Built        bool
	Moved        []string
	// Updates lists the remote entries whose commit or hash changed.
	Updates []Update
	// Warnings are for the user; the build succeeded despite them.
	Warnings []string
}

func writeAtomic(path string, data []byte) error {
	return imgstore.WriteFileAtomic(path, data)
}

// Build builds the image of opts.SourceDir into the store unless the source
// fingerprint equals the one of the last build and its image is still stored
// under every requested tag. Remote entries are checked against the lock file
// without the network; they are resolved only when a build is needed, when no
// lock record exists, or with opts.Update.
func Build(opts Options) (Result, error) {
	if opts.Resolver != nil {
		// One session per build: each git mirror is fetched at most once.
		r := opts.Resolver.WithSession()
		opts.Resolver = &r
	}
	j, err := newJob(opts)
	if err != nil {
		return Result{}, err
	}
	defer j.close()
	res := Result{Name: j.name, ImageVersion: j.f.ImageVersion, Tags: j.tags, Warnings: j.warnings}
	if !(opts.Update && j.remote) {
		current := j.withRestored(j.lock)
		if done, err := j.upToDate(current, &res); done || err != nil {
			if done {
				err = j.writeLock(current)
			}
			return res, err
		}
	}
	var rs *resolution
	if j.remote {
		if rs, err = resolve(context.Background(), opts.Resolver, j.f, j.lock, opts.Update); err != nil {
			return Result{}, err
		}
		j.recordRestored(rs)
		res.Updates = rs.updates
		if done, err := j.upToDate(rs.lock, &res); done || err != nil {
			if done {
				err = j.writeLock(rs.lock)
			}
			return res, err
		}
	}
	return j.publish(rs, res)
}

// writeLock stores lk when it differs from the lock file read at the start.
func (j *job) writeLock(lk lock.File) error {
	if j.lockPath == "" || !lockChanged(j.lock, lk) {
		return nil
	}
	return lk.Write(j.lockPath)
}

// job is the state of one Build call.
type job struct {
	opts     Options
	dir      string
	name     string
	f        *tfile.File
	tags     []string
	cache    *statCache
	remote   bool
	lock     lock.File
	lockPath string
	unlock   func()
	restore  *restorer
	warnings []string
}

// close releases the lock-file lock.
func (j *job) close() {
	if j.unlock != nil {
		j.unlock()
	}
}

func newJob(opts Options) (*job, error) {
	dir, err := filepath.Abs(opts.SourceDir)
	if err != nil {
		return nil, err
	}
	name := opts.Name
	if name == "" {
		name = filepath.Base(dir)
	}
	if err := imgstore.CheckName(name); err != nil {
		return nil, err
	}
	j := &job{opts: opts, dir: dir, name: name,
		cache: loadCache(opts.Store.StatCachePath(name)), lock: lock.File{Version: 1, Entries: map[string]lock.Entry{}}}
	if err := j.load(); err != nil {
		j.close()
		return nil, err
	}
	return j, nil
}

// load reads the source, restoring its skills-lock.json skills, and opens
// the lock file when the source has remote or restored entries.
func (j *job) load() error {
	if err := j.confineLayers(); err != nil {
		return err
	}
	if err := j.prepareRestore(); err != nil {
		return err
	}
	loaded, err := tfile.LoadWith(j.dir, j.restore.remap())
	if rerr := j.restore.failure(); rerr != nil {
		return rerr
	}
	if err != nil {
		return err
	}
	j.f = loaded.ForTbox()
	if err := j.confineEntries(); err != nil {
		return err
	}
	if err := j.restore.prime(j.f); err != nil {
		return err
	}
	if j.tags, err = buildTags(j.opts.Tags, j.f.ImageVersion); err != nil {
		return err
	}
	j.warnings = j.sourceWarnings(loaded)
	j.remote = firstRemote(j.f) != "" || len(j.restore.lockEntries()) > 0
	if firstRemote(j.f) == "" {
		return nil
	}
	if j.opts.Resolver == nil {
		return noResolverError(j.f)
	}
	return j.openLock()
}

// openLock takes the lock-file lock and reads the lock file. It holds the
// lock from the read to the write so two builds of one source cannot
// overwrite each other's records. A second call does nothing.
func (j *job) openLock() error {
	if j.lockPath != "" {
		return nil
	}
	lockPath := lock.PathFor(j.dir, j.opts.Resolver.Home)
	sum := sha256.Sum256([]byte(lockPath))
	lockFile := filepath.Join(j.opts.Resolver.Home.CacheDir(), "locks", "lockfile-"+hex.EncodeToString(sum[:])+".lock")
	unlock, err := flock.Lock(lockFile)
	if err != nil {
		return err
	}
	j.unlock = unlock
	if j.lock, err = lock.Read(lockPath); err != nil {
		return err
	}
	j.lockPath = lockPath
	return nil
}

// upToDate computes the fingerprint from lk and reports whether the stored
// image already matches it. An unknown fingerprint is never up to date.
func (j *job) upToDate(lk lock.File, res *Result) (bool, error) {
	fp, err := fingerprint(j.f, j.cache, &lk, j.restore.restoredKeys())
	if errors.Is(err, errUnknown) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := j.cache.save(j.opts.Store.StatCachePath(j.name)); err != nil {
		return false, err
	}
	digest, ok := upToDate(j.opts.Store, j.name, fp, j.tags)
	res.Digest = digest
	return ok, nil
}

// publish assembles and stores the image, then writes the lock and records
// the fingerprint computed from that lock.
func (j *job) publish(rs *resolution, res Result) (Result, error) {
	lk := j.lock
	if rs != nil {
		lk = rs.lock
	}
	fp, err := fingerprint(j.f, j.cache, &lk, j.restore.restoredKeys())
	if err != nil {
		return Result{}, err
	}
	if err := j.cache.save(j.opts.Store.StatCachePath(j.name)); err != nil {
		return Result{}, err
	}
	img, err := assemble(j.f, j.name, j.dir, j.opts.now(), rs)
	if err != nil {
		return Result{}, err
	}
	if res.Moved, err = j.opts.Store.Publish(j.name, img, j.tags); err != nil {
		return Result{}, err
	}
	if rs != nil {
		if err := j.writeLock(rs.lock); err != nil {
			return Result{}, err
		}
	}
	if err := j.opts.Store.SetFingerprint(j.name, fp, img.Manifest.Digest); err != nil {
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

func assemble(f *tfile.File, name, dir string, builtAt time.Time, rs *resolution) (archive.Image, error) {
	skills := make([]skill.Prepared, 0, len(f.Skills))
	for _, entry := range f.Skills {
		abs, source, category, err := rs.skillDir(entry, f.Remap)
		if err != nil {
			return archive.Image{}, err
		}
		prepared, err := skill.Prepare(abs, source, category)
		if err == nil {
			err = rs.verifySkill(entry, prepared.Metadata.TreeSHA256)
		}
		if err != nil {
			return archive.Image{}, fmt.Errorf("skill %s: %w", skillLabel(entry), err)
		}
		skills = append(skills, prepared)
	}
	prompts, err := promptLayers(f, rs)
	if err != nil {
		return archive.Image{}, err
	}
	role, err := tfile.RoleRead(dir)
	if err != nil {
		return archive.Image{}, err
	}
	return archive.Assemble(name, f.ImageVersion, builtAt, skills, prompts, role)
}

func promptLayers(f *tfile.File, rs *resolution) ([]archive.PromptLayer, error) {
	var layers []archive.PromptLayer
	skillDirs := f.SkillDirs()
	for _, entry := range f.Prompts {
		abs, source, category, err := rs.promptFile(entry, f.Remap, skillDirs)
		if err != nil {
			return nil, err
		}
		body, err := fsafe.ReadFileNoFollow(abs, maxPromptFile)
		if err != nil {
			return nil, fmt.Errorf("prompt %s: %w", promptLabel(entry), err)
		}
		if err := rs.verifyPrompt(entry, body); err != nil {
			return nil, fmt.Errorf("prompt %s: %w", promptLabel(entry), err)
		}
		layers = append(layers, archive.PromptLayer{Source: source, Category: category, Base: filepath.Base(abs), Body: body})
	}
	return layers, nil
}
