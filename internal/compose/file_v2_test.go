package compose

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const goodV2YAML = `version: 2
tariboy: {min: 0.110.0, max: 0.120.0}
stores:
  team: {git: https://example.com/store.git, ref: main}
  local: {path: ./store}
workflows:
  development: {store: team, source: development, version: 1.2.0}
  review: {path: ./workflows/review}
images:
  developer: {store: team, source: developer}
  reviewer: {path: ./images/reviewer}
groups:
  dev: {lead: dev-developer}
agents:
  dev-developer: {image: developer:latest, group: dev}
  dev-reviewer: {image: reviewer:latest, group: dev}
task_queues:
  DEV:
    name: Development
    workflow: team/development
    pools: {developers: [dev-developer], reviewers: [dev-reviewer]}
`

func parseV2(t *testing.T, doc string) File {
	t.Helper()
	f, err := Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return f
}

func problemTexts(f File) []string {
	var out []string
	for _, err := range f.problems() {
		out = append(out, err.Error())
	}
	return out
}

func TestVersion2FileIsValid(t *testing.T) {
	t.Parallel()
	f := parseV2(t, goodV2YAML)
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := f.Stores["team"]; got.Git != "https://example.com/store.git" || got.Ref != "main" {
		t.Fatalf("stores.team = %+v", got)
	}
	if got := f.Workflows["development"]; got.Store != "team" || got.Source != "development" || got.Version != "1.2.0" {
		t.Fatalf("workflows.development = %+v", got)
	}
	if got := f.Images["reviewer"].BuildPath(); got != "./images/reviewer" {
		t.Fatalf("reviewer BuildPath = %q", got)
	}
	c, err := f.VersionConstraint()
	if err != nil || c.String() != ">= 0.110.0, <= 0.120.0" {
		t.Fatalf("VersionConstraint = %v, %v", c, err)
	}
}

func TestVersion2ParseIsStrict(t *testing.T) {
	t.Parallel()
	for _, doc := range []string{
		"version: 2\nstores:\n  team: {git: x, branch: main}\n",
		"version: 2\nworkflows:\n  w: {path: ./w, digest: x}\n",
		"version: 2\nimages:\n  i: {path: ./i, digest: x}\n",
		"version: 2\ntariboy: {min: 0.1.0, exact: 0.1.0}\n",
	} {
		if _, err := Parse([]byte(doc)); err == nil || !strings.Contains(err.Error(), "not found in type") {
			t.Errorf("Parse(%q) = %v; want an unknown field error", doc, err)
		}
	}
}

func TestVersion1RejectsVersion2SectionsWithAHint(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ doc, want string }{
		{"version: 1\ntariboy: {min: 0.1.0}\n", "the tariboy key needs version: 2"},
		{"version: 1\nstores:\n  team: {path: ./s}\n", "the stores section needs version: 2"},
		{"version: 1\nimages:\n  i: {path: ./i}\n", `image "i" uses path, store, source, tag or version, which need version: 2`},
	} {
		f := parseV2(t, tc.doc)
		if err := f.Validate(); err == nil || err.Error() != tc.want {
			t.Errorf("Validate(%q) = %v; want %q", tc.doc, err, tc.want)
		}
	}
	_, err := Parse([]byte("version: 1\nworkflows:\n  w: {path: ./w}\n"))
	if err == nil || !strings.Contains(err.Error(), "declare version: 2") {
		t.Errorf("version 1 workflows error = %v; want a version: 2 hint", err)
	}
}

func TestVersion2SourceProblems(t *testing.T) {
	t.Parallel()
	f := parseV2(t, `version: 2
tariboy: {min: 0.120.0, max: 0.110.0}
stores:
  both: {git: https://example.com/s.git, path: ./s}
  none: {}
  pathref: {path: ./s, ref: main}
workflows:
  twosrc: {path: ./w, store: both}
  nosrc: {source: x}
  badver: {store: both, version: v1}
  ghost: {store: missing}
images:
  ctx: {context: ./ctx}
  pathver: {path: ./i, version: 1.0.0}
`)
	want := []string{
		`tariboy: min 0.120.0 is greater than max 0.110.0`,
		`store "both" needs exactly one source: git or path`,
		`store "none" needs exactly one source: git or path`,
		`store "pathref" ref needs git`,
		`workflow "badver" version: invalid version "v1" (want MAJOR.MINOR.PATCH[-PRERELEASE])`,
		`workflow "ghost" references undeclared store "missing"`,
		`workflow "nosrc" needs exactly one source: path or store`,
		`workflow "nosrc" source needs store`,
		`workflow "twosrc" needs exactly one source: path or store`,
		`image "ctx" context is version 1 syntax; use path`,
	}
	if got := problemTexts(f); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A version 2 entry may set tag and version with either source; a git Store
// takes only an https URL; a queue's Store prefix must match its entry.
func TestVersion2TagGitAndPrefixProblems(t *testing.T) {
	t.Parallel()
	f := parseV2(t, `version: 2
stores:
  team: {git: https://example.com/store.git}
  other: {git: https://example.com/other.git}
  plain: {git: http://example.com/s.git}
  sshurl: {git: ssh://git@example.com/s.git}
  scp: {git: "git@example.com:s.git"}
  creds: {git: "https://user:pw@example.com/s.git"}
  query: {git: "https://example.com/s.git?x=1"}
  nopath: {git: https://example.com}
workflows:
  development: {store: team, tag: 1.2.0, version: 1.2.0}
  review: {path: ./review, version: 0.1.0}
  badtag: {path: ./w, tag: stable}
images:
  developer: {store: team, tag: stable, version: 1.0.0}
  reviewer: {path: ./reviewer, tag: v2, version: 2.0.0}
  dots: {path: ./d, tag: ..}
  badtag: {path: ./d, tag: "a b"}
task_queues:
  DEV:
    name: Development
    workflow: other/development
  REV:
    name: Review
    workflow: review:0.1.0
`)
	want := []string{
		`store "creds" git must be an https URL without credentials, query or fragment`,
		`store "nopath" git must be an https URL without credentials, query or fragment`,
		`store "plain" git must be an https URL without credentials, query or fragment`,
		`store "query" git must be an https URL without credentials, query or fragment`,
		`store "scp" git must be an https URL without credentials, query or fragment`,
		`store "sshurl" git must be an https URL without credentials, query or fragment`,
		`workflow "badtag" tag "stable": want latest or a version`,
		`image "badtag" invalid tag "a b"`,
		`image "dots" invalid tag ".."`,
		`task queue "DEV" workflow store "other" differs from store "team" of workflow "development"`,
	}
	if got := problemTexts(f); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := f.Images["reviewer"]; got.Tag != "v2" || got.Version != "2.0.0" {
		t.Fatalf("images.reviewer = %+v", got)
	}
}

// A version 1 image keeps context; tag is version 2 syntax.
func TestVersion1ImageTagNeedsVersion2(t *testing.T) {
	t.Parallel()
	f := parseV2(t, "version: 1\nimages:\n  i: {context: ./i, tag: x}\n")
	if err := f.Validate(); err == nil || err.Error() != `image "i" uses path, store, source, tag or version, which need version: 2` {
		t.Fatalf("Validate = %v", err)
	}
	if err := parseV2(t, "version: 1\nimages:\n  i: {context: ./i}\n").Validate(); err != nil {
		t.Fatalf("version 1 context: %v", err)
	}
}

func TestVersion2ReferenceProblems(t *testing.T) {
	t.Parallel()
	f := parseV2(t, `version: 2
images:
  developer: {path: ./developer}
groups:
  dev: {}
agents:
  a: {image: ghost-image:latest, group: ghost-group}
  b: {image: developer:latest}
task_queues:
  DEV:
    name: Development
    workflow: nostore/development
    pools: {developers: [ghost-agent]}
  OPS:
    name: Operations
    workflow: ghost-workflow
`)
	want := []string{
		`agent "a" references undeclared group "ghost-group"`,
		`agent "a" references undeclared image "ghost-image"`,
		`task queue "DEV" pool "developers" references undeclared agent "ghost-agent"`,
		`task queue "DEV" references undeclared store "nostore"`,
		`task queue "DEV" references undeclared workflow "development"`,
		`task queue "OPS" references undeclared workflow "ghost-workflow"`,
	}
	if got := problemTexts(f); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestVersion1KeepsExternalReferences pins that the version 2 reference rules
// do not apply to version 1: an agent image and a queue workflow may name
// objects outside the file.
func TestVersion1KeepsExternalReferences(t *testing.T) {
	t.Parallel()
	f := parseV2(t, "version: 1\nagents:\n  a: {image: basic:latest}\ntask_queues:\n  DEV: {name: Development, workflow: official/development}\n")
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestVersion1MarshalHasNoVersion2Keys(t *testing.T) {
	t.Parallel()
	out, err := yaml.Marshal(File{Version: 1, Images: map[string]ImageSpec{"i": {Context: "./images/i"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tariboy", "stores", "workflows", "path", "store", "source"} {
		if strings.Contains(string(out), key+":") {
			t.Fatalf("marshaled version 1 file has %s:\n%s", key, out)
		}
	}
}
