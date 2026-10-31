import { useCallback, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { DeleteAgentButton } from "@/components/AgentControls";
import { useAgentName, useAgentStatus } from "@/lib/agent";
import { useApi } from "@/components/apiContext";
import type { AgentStatus, AgentView } from "@/lib/api";
import { useMultiHost } from "@/lib/navRoutes";
import { sectionOfKey } from "./configuration/registry";
import { countDirtyByGroup, groupOfKey, isStopped } from "./configuration/configurationPageModel";
import { useConfigDraft } from "./configuration/useConfigDraft";
import { useConfigSources } from "./configuration/useConfigSources";
import { buildUnits, readBaseline } from "./configuration/commits";
import { ConfigSaveRow, SettingsSection } from "./configuration/ConfigControls";
import { SettingsGroups } from "./configuration/SettingsGroups";
import { AdvancedSettings } from "./configuration/AdvancedSettings";
import { imageHref } from "./configuration/generalImageModel";
import TabPage from "@/components/TabPage";
import { FirstVisitHint } from "@/components/onboarding/FirstVisitHint";

export default function AgentConfigurationTab({
  base,
  refresh,
}: {
  /** The agent workspace route, for the links under Advanced → Other views. */
  base: string;
  refresh: () => void;
}) {
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
      base={base}
      refresh={refresh}
    />
  );
}

// About names what the agent runs and where; the image links to the Library.
function About({ view }: { view: AgentView }) {
  const target = useApi().target;
  const multiHost = useMultiHost();
  return (
    <SettingsSection id="about" title="About">
      <p className="text-sm">
        <span className="text-muted-foreground">image</span>{" "}
        {view.image ? (
          <Link
            className="font-mono text-primary hover:underline"
            to={imageHref(target?.id ?? "", view.image, multiHost)}
          >
            {view.image}
          </Link>
        ) : (
          "—"
        )}
        <span className="text-muted-foreground"> · host</span> {target?.label || "Local"}
      </p>
    </SettingsSection>
  );
}

function ConfigurationPage({
  hostId,
  name,
  status,
  base,
  refresh,
}: {
  hostId: string;
  name: string;
  status: AgentStatus | null;
  base: string;
  refresh: () => void;
}) {
  const api = useApi();
  const navigate = useNavigate();
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
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [editingFolder, setEditingFolder] = useState(false);

  // A save that stops on a field shows that field: its Advanced disclosure or
  // the folder editor opens first, and focus moves once it has rendered.
  const onFocus = useCallback((key: string) => {
    if (sectionOfKey(key)?.advanced) setAdvancedOpen(true);
    if (sectionOfKey(key)?.id === "folder") setEditingFolder(true);
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
  const dirtyByGroup = countDirtyByGroup(draft.dirtyKeys);
  const section = { draft, sources, name, stopped: isStopped(status, view), refresh };

  return (
    <TabPage narrow>
      <ConfigSaveRow draft={draft} groups={[...dirtyByGroup.keys()]} />
      <FirstVisitHint hostId={hostId} agent={name} tab="settings" />
      {!view ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : (
        <>
          <SettingsGroups
            {...section}
            editingFolder={editingFolder}
            onEditFolder={() => setEditingFolder(true)}
          />
          <AdvancedSettings
            {...section}
            base={base}
            open={advancedOpen}
            onOpenChange={setAdvancedOpen}
            dirty={draft.dirtyKeys.filter((key) => groupOfKey(key) === "Advanced").length}
          />
          <About view={view} />
          <SettingsSection id="danger" title="Danger">
            <div>
              <DeleteAgentButton name={name} refresh={refresh} onDeleted={() => navigate("/")} />
            </div>
          </SettingsSection>
        </>
      )}
    </TabPage>
  );
}
