package api

import (
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const MaxFileUploadBytes int64 = 1 << 30

type UploadResult struct {
	Path  string `json:"path"`
	Abs   string `json:"abs"`
	Bytes int64  `json:"bytes"`
}

// SaveUploadedFile streams one confined, owner-only file into the server's
// shared files directory and removes partial output on every failure.
func SaveUploadedFile(baseDir, name string, body io.Reader, maxBytes int64) (result UploadResult, err error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || strings.ContainsFunc(name, unicode.IsControl) {
		return result, UserError{Code: "bad_path", Msg: "name must be a file name without directories or control characters"}
	}
	base, err := filepath.Abs(baseDir)
	if err != nil {
		return result, err
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return result, err
	}
	defer root.Close()
	if err := root.MkdirAll("files", 0o700); err != nil {
		return result, err
	}
	dir := filepath.Join("files", rand.Text())
	if err := root.Mkdir(dir, 0o700); err != nil {
		return result, err
	}
	path := filepath.Join(dir, name)
	saved := false
	defer func() {
		if !saved {
			_ = root.Remove(path)
			_ = root.Remove(dir)
		}
	}()
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return result, err
	}
	written, copyErr := io.Copy(file, io.LimitReader(body, maxBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return result, copyErr
	}
	if written > maxBytes {
		return result, UserError{Code: "too_large", Msg: "file exceeds 1 GiB", Status: http.StatusRequestEntityTooLarge}
	}
	if closeErr != nil {
		return result, closeErr
	}
	saved = true
	return UploadResult{Path: path, Abs: filepath.Join(base, path), Bytes: written}, nil
}
