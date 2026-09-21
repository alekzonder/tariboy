package image

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// Publication remembers where a set of tags pointed and which content existed
// before a build, so a caller whose later metadata write fails can put the
// store back exactly as it was. It is in-process only: content is never
// destroyed, and an interrupted process leaves valid content behind that the
// next build simply replaces.
type Publication struct {
	store   *Store
	tags    map[Ref]string
	content map[string]bool
}

// BeginPublication records the current state of the refs a build will move.
func (s *Store) BeginPublication(refs []Ref) (*Publication, error) {
	p := &Publication{store: s, tags: make(map[Ref]string, len(refs)), content: map[string]bool{}}
	for _, ref := range refs {
		if _, ok := p.tags[ref]; ok {
			return nil, errors.New("duplicate publication ref")
		}
		id, err := s.Resolve(ref)
		if err != nil {
			id = ""
		}
		p.tags[ref] = id
		if err := p.recordContent(ref.Name); err != nil {
			return nil, err
		}
	}
	return p, nil
}

func (p *Publication) recordContent(name string) error {
	entries, err := os.ReadDir(p.store.refsDir(name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".tar.gz") {
			p.content[filepath.Join(name, entry.Name())] = true
		}
	}
	return nil
}

// Restore puts every recorded tag back and deletes only content this
// publication introduced. Content that existed before — including generations
// pinned by agents — is left alone.
func (p *Publication) Restore() error {
	var failures []error
	for ref, id := range p.tags {
		var err error
		if id == "" {
			err = os.Remove(p.store.tagPath(ref))
			if os.IsNotExist(err) {
				err = nil
			}
		} else {
			err = p.store.SetTag(ref, id)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	names := map[string]bool{}
	for ref := range p.tags {
		names[ref.Name] = true
	}
	for name := range names {
		entries, err := os.ReadDir(p.store.refsDir(name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
				continue
			}
			if p.content[filepath.Join(name, entry.Name())] {
				continue
			}
			if err := os.Remove(filepath.Join(p.store.refsDir(name), entry.Name())); err != nil {
				failures = append(failures, err)
			}
		}
		_ = os.Remove(p.store.tagsDir(name))
		_ = os.Remove(p.store.refsDir(name))
		_ = os.Remove(p.store.nameDir(name))
	}
	return errors.Join(failures...)
}
