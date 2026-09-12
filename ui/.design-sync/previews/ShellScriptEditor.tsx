import { ShellScriptEditor } from "tariboy-ui";

// ShellScriptEditor is fully injected: `load`/`save` are props (SettingsPage
// passes the host-global script, AgentSettings the per-agent one), so the real
// component renders real content here with no daemon in the loop.
//
// `load` identity is the readiness key (`ready = loaded === load`), so every
// loader below is a module-scope constant — an inline arrow would re-fire the
// effect each render and pin the card in its disabled/loading state.

const GLOBAL_SCRIPT = `# Sourced before every agent iteration on this host.
export PATH="$HOME/.local/bin:$PATH"
export TB_CACHE="/home/agent/.cache/tariboy"

# Toolchain pinned for the worker:v2 image.
. /opt/toolchains/go-1.23/activate

# Keep git quiet in non-interactive iterations.
git config --global advice.detachedHead false
`;

const AGENT_SCRIPT = `# builder — sourced after the global script.
cd /home/agent/github/tariboy
export TB_TASK=TB-142
export GOFLAGS=-mod=mod

# Warm the build cache so the first iteration step is not a cold compile.
go build ./... >/dev/null 2>&1 || true
`;

const loadGlobal = () => Promise.resolve(GLOBAL_SCRIPT);
const loadAgent = () => Promise.resolve(AGENT_SCRIPT);
const loadFailed = () =>
  Promise.reject(new Error("host build-01 is not ready: dial tcp 10.0.4.11:7777: connection refused"));
const save = () => Promise.resolve();

const Frame = ({ children }: { children: React.ReactNode }) => (
  <div style={{ maxWidth: 660 }}>{children}</div>
);

// Settings → Advanced on a host: the script every agent on it sources.
export const GlobalScript = () => (
  <Frame>
    <ShellScriptEditor
      title="Global Agent Shell Script"
      description="Bash commands sourced before every agent iteration on this host."
      load={loadGlobal}
      save={save}
    />
  </Frame>
);

// The per-agent editor on the agent Settings tab.
export const AgentScript = () => (
  <Frame>
    <ShellScriptEditor
      title="Agent Shell Script"
      description="Bash commands sourced after the global script before this agent starts an iteration."
      load={loadAgent}
      save={save}
    />
  </Frame>
);

// A rejected load: the textarea stays disabled (nothing to edit yet), Save is
// blocked, and the reason is reported under the field.
export const LoadFailed = () => (
  <Frame>
    <ShellScriptEditor
      title="Agent Shell Script"
      description="Bash commands sourced after the global script before this agent starts an iteration."
      load={loadFailed}
      save={save}
    />
  </Frame>
);
