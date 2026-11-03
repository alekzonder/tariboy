package image

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alekzonder/tariboy/internal/imagefile"
)

// BenchmarkInspectPinned measures the read an agent activation does on every
// iteration start.
//
// "typical" matches the techlead image of a developer store: 87 members and
// about 490 KiB uncompressed (150 KiB compressed). It is built from 20 skills
// of 4 text files of 6 KiB each. "limit" is close to the 64 MiB compressed
// archive limit: 8 skills with one 7.5 MiB random file each.
func BenchmarkInspectPinned(b *testing.B) {
	for _, tc := range []struct {
		name          string
		skills, files int
		fileBytes     int
		random        bool
	}{
		{name: "typical", skills: 20, files: 3, fileBytes: 6 << 10},
		{name: "limit", skills: 8, files: 1, fileBytes: 15 << 19, random: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			source := b.TempDir()
			rng := rand.New(rand.NewSource(1))
			src := &imagefile.V2{SchemaVersion: 2, Dir: source, ImageVersion: "1.0.0"}
			for i := 0; i < tc.skills; i++ {
				name := fmt.Sprintf("bench-skill-%02d", i)
				dir := filepath.Join(source, "skills", name)
				if err := os.MkdirAll(dir, 0o700); err != nil {
					b.Fatal(err)
				}
				skill := "---\nname: " + name + "\ndescription: Use this skill for benchmarks.\n---\n" + benchText(rng, tc.fileBytes)
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skill), 0o644); err != nil {
					b.Fatal(err)
				}
				for j := 0; j < tc.files; j++ {
					body := []byte(benchText(rng, tc.fileBytes))
					if tc.random {
						body = make([]byte, tc.fileBytes)
						rng.Read(body)
					}
					if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("ref-%d.md", j)), body, 0o644); err != nil {
						b.Fatal(err)
					}
				}
				src.Skills = append(src.Skills, imagefile.SkillEntry{Dir: "./skills/" + name})
			}
			store := &Store{Dir: b.TempDir()}
			ref := Ref{Name: "bench", Tag: "latest"}
			manifest, err := BuildV2(src, imagefile.ResolveRoots{}, ref, store, time.Now, nil)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := store.InspectPinned(ref, manifest.Digest); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

var benchWords = strings.Fields("image agent skill digest archive manifest store build version tag layer prompt run check plan task review")

func benchText(rng *rand.Rand, size int) string {
	var out strings.Builder
	for out.Len() < size {
		out.WriteString(benchWords[rng.Intn(len(benchWords))])
		out.WriteByte(' ')
	}
	return out.String()[:size]
}
