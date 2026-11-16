package cli

import (
	"github.com/alekzonder/tariboy/tbox/internal/gitx"
	"github.com/alekzonder/tariboy/tbox/internal/paths"
	"github.com/alekzonder/tariboy/tbox/internal/source"
	"github.com/alekzonder/tariboy/tbox/internal/stores"
)

// UseLocalGit makes the registry and the resolver accept local repositories.
// The end-to-end child process calls it; production code never does.
func UseLocalGit() {
	newRegistry = func(home paths.Home, cwd string) stores.Registry {
		return stores.Registry{Home: home, Cwd: cwd, Git: gitx.Git{AllowLocal: true}}
	}
	newResolver = func(home paths.Home) *source.Resolver {
		return &source.Resolver{Home: home, Git: gitx.Git{AllowLocal: true}}
	}
}
