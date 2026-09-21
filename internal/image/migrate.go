package image

import (
	"os"
	"path/filepath"
	"strings"
)

// legacyHistoryDirs are the pre-ref-model directories that retained previous
// generations of a moving ref. Their archives become ordinary content.
var legacyHistoryDirs = []string{".mutable", ".managed"}

// Migrate converts a pre-ref-model store — one archive per tag, plus mutable
// markers, digest sidecars, retained generations and publication journals — to
// the refs/tags layout. Each legacy archive keeps its content digest as its ref
// id, so digests already pinned on agents continue to resolve. Migration is
// idempotent and one-way.
func (s *Store) Migrate() error {
	entries, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if err := s.migrateImage(entry.Name()); err != nil {
			return err
		}
	}
	for _, legacy := range legacyHistoryDirs {
		if err := s.migrateHistory(filepath.Join(s.Dir, legacy)); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(filepath.Join(s.Dir, ".publications")); err != nil {
		return err
	}
	return nil
}

// migrateImage moves <name>/<tag>.tar.gz to <name>/refs/<digest>.tar.gz and
// writes the tag pointer, dropping the digest and mutable sidecars.
func (s *Store) migrateImage(name string) error {
	entries, err := os.ReadDir(s.nameDir(name))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		file := entry.Name()
		switch {
		case strings.HasSuffix(file, ".tar.gz"):
			if err := s.adoptLegacyArchive(name, strings.TrimSuffix(file, ".tar.gz"), filepath.Join(s.nameDir(name), file)); err != nil {
				return err
			}
		case strings.HasSuffix(file, ".digest"), strings.HasSuffix(file, ".mutable"), strings.HasSuffix(file, ".tmp"):
			if err := os.Remove(filepath.Join(s.nameDir(name), file)); err != nil {
				return err
			}
		}
	}
	return nil
}

// migrateHistory adopts retained generations from <root>/<name>/<tag>/<digest>.tar.gz.
func (s *Store) migrateHistory(root string) error {
	names, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, name := range names {
		if !name.IsDir() {
			continue
		}
		tags, err := os.ReadDir(filepath.Join(root, name.Name()))
		if err != nil {
			return err
		}
		for _, tag := range tags {
			if !tag.IsDir() {
				continue
			}
			archives, err := os.ReadDir(filepath.Join(root, name.Name(), tag.Name()))
			if err != nil {
				return err
			}
			for _, archive := range archives {
				if archive.IsDir() || !strings.HasSuffix(archive.Name(), ".tar.gz") {
					continue
				}
				if err := s.adoptLegacyArchive(name.Name(), "", filepath.Join(root, name.Name(), tag.Name(), archive.Name())); err != nil {
					return err
				}
			}
		}
	}
	return os.RemoveAll(root)
}

// adoptLegacyArchive stores one legacy archive under its content digest and,
// when tag is not empty, points that tag at it.
func (s *Store) adoptLegacyArchive(name, tag, path string) error {
	id, err := fileDigest(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.refsDir(name), 0o700); err != nil {
		return err
	}
	target := s.contentPath(name, id)
	if _, err := os.Stat(target); err != nil {
		if err := os.Rename(path, target); err != nil {
			return err
		}
	} else if err := os.Remove(path); err != nil {
		return err
	}
	if err := syncDirectory(s.refsDir(name)); err != nil {
		return err
	}
	if tag == "" {
		return nil
	}
	return s.SetTag(Ref{Name: name, Tag: tag}, id)
}
