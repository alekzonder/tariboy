package loop

import (
	"github.com/alekzonder/tariboy/internal/agent"
	"github.com/alekzonder/tariboy/internal/agentdir"
	"github.com/alekzonder/tariboy/internal/harness"
)

// instantHooks writes the harness plugin whose hooks run tmail instant and
// returns it. It loads the plugin only for a harness with such hooks (claude,
// codex, opencode), a non-bare image and an agent with the mail capability.
// cfg is the config the adapter's Command got. A missing tmail, an old or
// unreadable harness version, a mode the adapter refuses or a failed write
// only logs a warning: the iteration starts without the hooks and the letter
// reaches the next prompt. The hooks call the absolute tmail path, so a PATH
// change in the agent shell prologue does not affect them.
func (r *ShimRunner) instantHooks(ag agent.Agent, l agentdir.Layout, adapter harness.Adapter, env []string, cwd string, cfg harness.Config) (harness.InstantMailHooks, bool) {
	hooker, ok := adapter.(harness.InstantMailHooker)
	if !ok || cfg.Bare || !hasMailPlugin(ag.Plugins) {
		return harness.InstantMailHooks{}, false
	}
	warn := func(reason string, err error) (harness.InstantMailHooks, bool) {
		r.cfg.Logger.Warn("instant mail hooks not loaded; instant letters wait for the next iteration",
			"agent", ag.Name, "harness", adapter.Type(), "reason", reason, "err", err)
		return harness.InstantMailHooks{}, false
	}
	tmail, err := harness.FindExecutable("tmail", env, cwd)
	if err != nil {
		return warn("tmail not found", err)
	}
	dir := l.InstantHooksDir(adapter.Type())
	hooks, err := hooker.InstantMailHooks(tmail, dir, cfg)
	if err != nil {
		return warn("build hooks", err)
	}
	executable, err := harness.FindExecutable(adapter.Executable(), env, cwd)
	if err != nil {
		return warn("harness not found", err)
	}
	if err := harness.CheckMinimumVersion(executable, env, hooks.MinimumVersion); err != nil {
		return warn("harness version", err)
	}
	if err := agentdir.WritePluginFiles(dir, hooks.Files); err != nil {
		return warn("write hooks", err)
	}
	return hooks, true
}

// launchInstantHooks returns the instant mail hooks whose Args load them.
// OpenCode loads its plugin from the configuration overlay instead, which
// prepareOpenCode builds, and runs tmail without a shell.
func (r *ShimRunner) launchInstantHooks(ag agent.Agent, l agentdir.Layout, adapter harness.Adapter, env []string, cwd string, cfg harness.Config) harness.InstantMailHooks {
	if adapter.Type() == "opencode" {
		return harness.InstantMailHooks{}
	}
	hooks, _ := r.instantHooks(ag, l, adapter, env, cwd, cfg)
	return hooks
}

// hasMailPlugin reports whether plugins hold mail, or messages, its name in
// images built before the mailbox.
func hasMailPlugin(plugins []string) bool {
	for _, p := range plugins {
		if p == "mail" || p == "messages" {
			return true
		}
	}
	return false
}
