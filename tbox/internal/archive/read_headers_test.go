package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"strings"
	"testing"
)

// rawMember is one tar header written as given, with an optional body. A
// zero Mode means 0600; a body shorter than Size ends the stream there.
type rawMember struct {
	hdr  tar.Header
	body []byte
}

// rawArchive writes the members with their headers unchanged, ending the
// stream without a tar footer when the last body is short.
func rawArchive(t *testing.T, members []rawMember) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	short := false
	for _, m := range members {
		h := m.hdr
		if h.Mode == 0 {
			h.Mode = 0o600
		}
		if err := tw.WriteHeader(&h); err != nil {
			t.Fatal(err)
		}
		if int64(len(m.body)) == h.Size {
			if _, err := tw.Write(m.body); err != nil {
				t.Fatal(err)
			}
		} else {
			short = true
			break
		}
	}
	if short {
		_ = tw.Flush()
	} else if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func reg(name string, body string) rawMember {
	return rawMember{hdr: tar.Header{Name: name, Typeflag: tar.TypeReg, Size: int64(len(body))}, body: []byte(body)}
}

// TestReadRejectsRawHeaders feeds Read archives whose tar headers break the
// member rules of internal/image portable_validate.go.
func TestReadRejectsRawHeaders(t *testing.T) {
	many := make([]rawMember, 0, maxMembers+1)
	for i := 0; i <= maxMembers; i++ {
		many = append(many, reg(fmt.Sprintf("f/%d", i), ""))
	}
	half := maxExpanded/2 + 1
	cases := []struct {
		name    string
		members []rawMember
		want    string
	}{
		{"dot-dot path", []rawMember{reg("../evil", "x")}, "invalid image archive member path"},
		{"inner dot-dot", []rawMember{reg("skills/../../evil", "x")}, "invalid image archive member path"},
		{"absolute path", []rawMember{reg("/etc/evil", "x")}, "invalid image archive member path"},
		{"unclean path", []rawMember{reg("skills//a", "x")}, "invalid image archive member path"},
		{"backslash path", []rawMember{reg(`skills\a`, "x")}, "invalid image archive member path"},
		{"path over 512 bytes", []rawMember{reg(strings.Repeat("a", maxPathBytes+1), "x")}, "invalid image archive member path"},
		{"symlink", []rawMember{{hdr: tar.Header{Name: "manifest.json", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}}}, "regular files only"},
		{"hardlink", []rawMember{{hdr: tar.Header{Name: "manifest.json", Typeflag: tar.TypeLink, Linkname: "other"}}}, "regular files only"},
		{"directory", []rawMember{{hdr: tar.Header{Name: "manifest.json", Typeflag: tar.TypeDir, Mode: 0o700}}}, "regular files only"},
		{"character device", []rawMember{{hdr: tar.Header{Name: "dev", Typeflag: tar.TypeChar, Devmajor: 1, Devminor: 3}}}, "regular files only"},
		{"fifo", []rawMember{{hdr: tar.Header{Name: "fifo", Typeflag: tar.TypeFifo}}}, "regular files only"},
		{"setuid mode", []rawMember{{hdr: tar.Header{Name: "a", Typeflag: tar.TypeReg, Mode: 0o4700}}}, "mode"},
		{"world-readable mode", []rawMember{{hdr: tar.Header{Name: "a", Typeflag: tar.TypeReg, Mode: 0o644}}}, "mode"},
		{"duplicate member", []rawMember{reg("a", "1"), reg("a", "2")}, "duplicate image archive member"},
		{"too many members", many, "too many files"},
		{"expanded size over the limit", []rawMember{
			{hdr: tar.Header{Name: "a", Typeflag: tar.TypeReg, Size: half}, body: make([]byte, half)},
			{hdr: tar.Header{Name: "b", Typeflag: tar.TypeReg, Size: half}},
		}, "expanded size limit"},
		{"declared size over the limit", []rawMember{{hdr: tar.Header{Name: "a", Typeflag: tar.TypeReg, Size: maxExpanded + 1}}}, "expanded size limit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Read(bytes.NewReader(rawArchive(t, c.members)))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
		})
	}
}
