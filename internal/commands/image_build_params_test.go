package commands

import (
	"errors"
	"reflect"
	"testing"

	"github.com/alekzonder/tariboy/internal/api"
	"github.com/alekzonder/tariboy/internal/registry"
)

func TestParseImageBuildParams(t *testing.T) {
	t.Parallel()
	commit := "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		name string
		p    registry.Params
		want imageBuildParams
		code string
	}{
		{name: "path and tags", p: registry.Params{"path": "/src", "name": "dev", "tag": []any{"1.0", "latest"}},
			want: imageBuildParams{name: "dev", path: "/src", tags: []string{"1.0", "latest"}}},
		{name: "single tag string", p: registry.Params{"path": "/src", "name": "dev", "tag": "1.0"},
			want: imageBuildParams{name: "dev", path: "/src", tags: []string{"1.0"}}},
		{name: "store selector", p: registry.Params{"source": " store/dev ", "tag": []any{}},
			want: imageBuildParams{selector: "store/dev", tags: nil}},
		{name: "provenance", p: registry.Params{"path": "/src", "name": "dev", "tag": []any{}, "repository-id": " repo ", "git-commit": commit},
			want: imageBuildParams{name: "dev", path: "/src", tags: nil, repositoryID: "repo", gitCommit: commit}},
		{name: "source and path", p: registry.Params{"source": "store/dev", "path": "/src"}, code: "bad_source"},
		{name: "no source or path", p: registry.Params{"name": "dev"}, code: "missing_path"},
		{name: "blank path", p: registry.Params{"path": "  ", "name": "dev"}, code: "missing_path"},
		{name: "repository without commit", p: registry.Params{"path": "/src", "tag": []any{}, "repository-id": "repo"}, code: "bad_provenance"},
		{name: "commit without repository", p: registry.Params{"path": "/src", "tag": []any{}, "git-commit": commit}, code: "bad_provenance"},
		{name: "short commit", p: registry.Params{"path": "/src", "tag": []any{}, "repository-id": "repo", "git-commit": "abc"}, code: "bad_provenance"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseImageBuildParams(tc.p)
			if tc.code != "" {
				var ue api.UserError
				if !errors.As(err, &ue) || ue.Code != tc.code {
					t.Fatalf("err = %v, want code %s", err, tc.code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("params = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestResolveImageBuildName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, inName string
		inTags       []string
		wantName     string
		wantTags     []string
		code         string
	}{
		{name: "explicit name", inName: "dev", inTags: []string{"1.0"}, wantName: "dev", wantTags: []string{"1.0"}},
		{name: "legacy ref tag", inTags: []string{"dev:1.0"}, wantName: "dev", wantTags: []string{"1.0"}},
		{name: "no name", inTags: []string{"latest", "1.0"}, code: "missing_name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name, tags, err := resolveImageBuildName(tc.inName, tc.inTags)
			if tc.code != "" {
				var ue api.UserError
				if !errors.As(err, &ue) || ue.Code != tc.code {
					t.Fatalf("err = %v, want code %s", err, tc.code)
				}
				return
			}
			if err != nil || name != tc.wantName || !reflect.DeepEqual(tags, tc.wantTags) {
				t.Fatalf("got %q %v %v, want %q %v", name, tags, err, tc.wantName, tc.wantTags)
			}
		})
	}
}
