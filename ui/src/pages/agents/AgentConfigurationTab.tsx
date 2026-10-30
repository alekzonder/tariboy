import { useCallback, useMemo, useState } from "react";
import { RunStateActions } from "@/components/AgentControls";
import { useAgentName, useAgentStatus } from "@/lib/agent";
import { useApi } from "@/components/apiContext";
import type { AgentStatus } from "@/lib/api";
import { BLOCKS, filterBlocks } from "./configuration/registry";
import { countDirtyByBlock, isStopped } from "./configuration/configurationPageModel";
import { ConfigEmptyState } from "./configuration/ConfigEmptyState";
import { useConfigDraft } from "./configuration/useConfigDraft";
import { useConfigSources } from "./configuration/useConfigSources";
import { buildUnits, readBaseline } from "./configuration/commits";
import { ConfigSaveBar, ConfigSearch } from "./configuration/ConfigControls";
import { ConfigurationBlocks } from "./configuration/ConfigurationBlocks";
import TabPage from "@/components/TabPage";
import { FirstVisitHint } from "@/components/onboarding/FirstVisitHint";

export default function AgentConfigurationTab({ refresh }: { refresh: () => void }) {
  const name = useAgentName();
  const { status } = useAgentStatus();
  // The host is the surrounding ApiProvider's. Its client keeps its identity
  // while the endpoint and token are unchanged, so host registry refreshes
  // cannot reload the sources over an in-progress draft.
  const targetId = useApi().target?.id;
  // One draft per scope (one agent on one host): a reconnect keeps the scope
  // and the draft; another agent or host starts clean.
  return (
    <ConfigurationPage
      key={`${targetId ?? ""}\u0000${name}`}
      hostId={targetId ?? ""}
      name={name}
      status={status}
      refresh={refresh}
    />
  );
}

function ConfigurationPage({
  hostId,
  name,
  status,
  refresh,
}: {
  hostId: string;
  name: string;
  status: AgentStatus | null;
  refresh: () => void;
}) {
  const api = useApi();
  const sources = useConfigSources(name, status);
  const { view, imageStatus } = sources;
  const baseline = useMemo(
    () =>
      readBaseline({
        view,
        imageStatus,
        script: sources.script.value,
        retention: sources.retention.value,
        secrets: sources.secrets.value,
      }),
    [view, imageStatus, sources.script.value, sources.retention.value, sources.secrets.value],
  );
  const units = useMemo(() => buildUnits(api, name), [api, name]);
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

  const dirtyByBlock = countDirtyByBlock(draft.dirtyKeys);
  const filtered = filterBlocks(query, available);
  const visible = filtered.blocks.filter((block) => !changedOnly || dirtyByBlock.has(block.id));
  const filtering = query.trim() !== "" || changedOnly;

  const stopped = isStopped(status, view);

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
      <FirstVisitHint hostId={hostId} agent={name} tab="configuration" />
      <ConfigSearch
        query={query}
        onQuery={setQuery}
        changedOnly={changedOnly}
        onChangedOnly={setChangedOnly}
        dirtyCount={draft.dirtyKeys.length}
        chips={available.map((block) => ({
          id: block.id,
          title: block.title,
          dirty: dirtyByBlock.get(block.id) ?? 0,
        }))}
        resultText={filtering ? `${visible.length} of ${available.length} blocks match` : ""}
      />
      {!view ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : visible.length === 0 ? (
        <ConfigEmptyState
          query={query}
          onClear={() => {
            setQuery("");
            setChangedOnly(false);
          }}
        />
      ) : (
        visible.map((block) => (
          <ConfigurationBlocks
            key={block.id}
            block={block}
            draft={draft}
            matched={filtered.matched}
            dirty={dirtyByBlock.get(block.id) ?? 0}
            sources={sources}
            name={name}
            stopped={stopped}
            refresh={refresh}
          />
        ))
      )}
      <ConfigSaveBar
        draft={draft}
        blockTitles={available
          .filter((block) => dirtyByBlock.has(block.id))
          .map((block) => block.title)}
      />
    </TabPage>
  );
}
