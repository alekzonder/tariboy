import { useCallback, useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { RunStateActions } from "@/components/AgentControls";
import { useAgentName, useAgentStatus } from "@/lib/agent";
import type { Daemon } from "@/lib/daemons";
import type { AgentStatus } from "@/lib/types";
import { BLOCKS, blockOfKey, filterBlocks } from "./configuration/registry";
import { useConfigDraft } from "./configuration/useConfigDraft";
import { useConfigSources } from "./configuration/useConfigSources";
import { buildUnits, readBaseline } from "./configuration/commits";
import { ConfigSaveBar, ConfigSearch } from "./configuration/ConfigControls";
import { ConfigurationBlocks } from "./configuration/ConfigurationBlocks";
import TabPage from "@/components/TabPage";

export default function AgentConfigurationTab({
  target,
  refresh,
}: {
  target: Daemon | null;
  refresh: () => void;
}) {
  const name = useAgentName();
  const { status } = useAgentStatus();
  const targetId = target?.id;
  const targetLabel = target?.label;
  const targetBaseURL = target?.baseURL;
  const targetToken = target?.token;
  // Host registry refreshes replace the descriptor object even when its API
  // endpoint is unchanged. Keep a stable request target so those parent
  // renders cannot reload the sources over an in-progress draft.
  const requestTarget = useMemo<Daemon | null>(
    () => targetId === undefined ? null : {
      id: targetId,
      label: targetLabel ?? targetId,
      baseURL: targetBaseURL ?? "",
      token: targetToken ?? "",
    },
    [targetBaseURL, targetId, targetLabel, targetToken],
  );
  // One draft per scope (one agent on one host): a reconnect keeps the scope
  // and the draft; another agent or host starts clean.
  return (
    <ConfigurationPage
      key={`${targetId ?? ""}\u0000${name}`}
      target={requestTarget}
      name={name}
      status={status}
      refresh={refresh}
    />
  );
}

function ConfigurationPage({
  target,
  name,
  status,
  refresh,
}: {
  target: Daemon | null;
  name: string;
  status: AgentStatus | null;
  refresh: () => void;
}) {
  const sources = useConfigSources(target, name, status);
  const { view, imageStatus } = sources;
  const baseline = useMemo(() => readBaseline({
    view,
    imageStatus,
    script: sources.script.value,
    retention: sources.retention.value,
    secrets: sources.secrets.value,
  }), [view, imageStatus, sources.script.value, sources.retention.value, sources.secrets.value]);
  const units = useMemo(() => buildUnits(target, name), [target, name]);
  const [query, setQuery] = useState("");
  const [changedOnly, setChangedOnly] = useState(false);

  // Older daemons omit the budget projection; then the block does not exist.
  const available = useMemo(
    () => BLOCKS.filter((block) => block.id !== "budgets" || !view || view.budget),
    [view],
  );
  // A save that stops on a field shows that field: search is cleared when it
  // hides the field's block, and focus moves once the block has rendered.
  const onFocus = useCallback((key: string) => {
    if (!document.getElementById(key)) {
      setQuery("");
      setChangedOnly(false);
    }
    window.setTimeout(() => {
      const element = document.getElementById(key);
      element?.scrollIntoView?.({ block: "center" });
      element?.focus();
    }, 0);
  }, []);
  const reload = useCallback(async () => {
    await sources.reloadAll();
    refresh();
  }, [sources, refresh]);
  const draft = useConfigDraft({ baseline, units, reload, onFocus });

  const dirtyByBlock = new Map<string, number>();
  for (const key of draft.dirtyKeys) {
    const id = blockOfKey(key);
    dirtyByBlock.set(id, (dirtyByBlock.get(id) ?? 0) + 1);
  }
  const filtered = filterBlocks(query, available);
  const visible = filtered.blocks.filter((block) => !changedOnly || dirtyByBlock.has(block.id));
  const filtering = query.trim() !== "" || changedOnly;

  const stopped = status
    ? status.state === "stopped"
    : view?.enabled === false || (view?.enabled === undefined && view?.state === "stopped");

  return (
    <TabPage narrow>
      <header className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h2 className="font-semibold">Configuration</h2>
          {view && (
            <p className="text-sm">
              <span className="text-muted-foreground">Master switch</span>{" "}
              <span data-testid="master-switch-state" className="font-medium">
                {view.enabled === false ? "Disabled" : "Enabled"}
              </span>
            </p>
          )}
        </div>
        <div className="space-y-1">
          <RunStateActions name={name} refresh={() => void reload()} />
          <p className="text-xs text-muted-foreground">Run actions apply immediately.</p>
        </div>
      </header>
      <ConfigSearch
        query={query}
        onQuery={setQuery}
        changedOnly={changedOnly}
        onChangedOnly={setChangedOnly}
        dirtyCount={draft.dirtyKeys.length}
        chips={available.map((block) => ({ id: block.id, title: block.title, dirty: dirtyByBlock.get(block.id) ?? 0 }))}
        resultText={filtering ? `${visible.length} of ${available.length} blocks match` : ""}
      />
      {!view ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : visible.length === 0 ? (
        <div className="rounded-lg border border-dashed p-6 text-center">
          <p className="text-sm font-medium">{query.trim() ? `No settings match “${query.trim()}”` : "No blocks have unsaved changes"}</p>
          <p className="mt-1 text-xs text-muted-foreground">Unsaved changes in hidden blocks are kept.</p>
          <Button variant="outline" size="sm" className="mt-3" onClick={() => { setQuery(""); setChangedOnly(false); }}>
            Clear search
          </Button>
        </div>
      ) : (
        visible.map((block) => (
          <ConfigurationBlocks
            key={block.id}
            block={block}
            draft={draft}
            matched={filtered.matched}
            dirty={dirtyByBlock.get(block.id) ?? 0}
            sources={sources}
            target={target}
            name={name}
            stopped={stopped}
            refresh={refresh}
          />
        ))
      )}
      <ConfigSaveBar
        draft={draft}
        blockTitles={available.filter((block) => dirtyByBlock.has(block.id)).map((block) => block.title)}
      />
    </TabPage>
  );
}
