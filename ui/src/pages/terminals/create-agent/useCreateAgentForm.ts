import { useState } from "react";
import { useHarnessCatalog } from "@/hooks/useHarnessCatalog";
import { runtimePresetGroups } from "@/lib/runtimePresets";
import { newAgentDraft, type AgentCreateDraft } from "../agentCreateDraft";
import type { CreateAgentDialogProps } from "../CreateAgentDialog";
import { createBlocker } from "./createBlocker";
import { useCloneAgentSource } from "./useCloneAgentSource";
import { useCreateAgentResources } from "./useCreateAgentResources";
import { useCreateAgentSubmit } from "./useCreateAgentSubmit";

export function useCreateAgentForm(props: CreateAgentDialogProps) {
  const { imageRef = "", hosts } = props;
  const [draft, setDraft] = useState<AgentCreateDraft>(() => newAgentDraft(imageRef));
  const [pluginDraft, setPluginDraft] = useState("");
  const updateDraft = <K extends keyof AgentCreateDraft>(key: K, value: AgentCreateDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };
  const source = useCloneAgentSource(props.cloneSource, setDraft);
  const resources = useCreateAgentResources(props, draft, setDraft, updateDraft);
  const { sourceState } = source;
  const {
    host,
    target,
    targetIsCurrent,
    targetIsUsable,
    targetImageAvailable,
    expectedManifestKey,
    manifestKey,
    manifestError,
    manifestIsCurrent,
    imagesError,
    defaultModel,
    defaultEffort,
  } = resources;
  // An unresolved host asks nothing: a null target would address the local
  // daemon instead of the selected one.
  const harnessCatalog = useHarnessCatalog(
    target,
    targetIsCurrent && targetIsUsable ? draft.harness : "",
  );
  const modelPresets = runtimePresetGroups({
    harness: draft.harness,
    field: "models",
    catalog: harnessCatalog,
    extras: [defaultModel, draft.model],
  });
  const effortPresets = runtimePresetGroups({
    harness: draft.harness,
    field: "efforts",
    catalog: harnessCatalog,
    model: draft.model,
    extras: [defaultEffort, draft.effort],
  });
  const hostEntry = hosts.find((entry) => entry.id === host);
  const blocker = createBlocker({
    sourceState,
    hostLabel: hostEntry?.label || host || "local",
    targetIsCurrent,
    imagesError,
    image: draft.image,
    imageAvailable: targetImageAvailable,
    manifestError: manifestKey === expectedManifestKey ? manifestError : "",
    manifestIsCurrent,
  });

  const submission = useCreateAgentSubmit(draft, resources, blocker, props);
  const formDisabled = submission.busy || submission.created !== null || sourceState === "loading";
  const changeHost = (value: string) => {
    if ((value === "__local__" ? "" : value) === host) return;
    resources.changeHost(value);
    submission.setSubmitError("");
  };
  const changeImage = (value: string) => {
    resources.changeImage(value);
    submission.setSubmitError("");
  };
  const addPlugin = () => {
    const plugin = pluginDraft.trim();
    if (plugin && !draft.plugins.includes(plugin)) {
      updateDraft("plugins", [...draft.plugins, plugin]);
    }
    setPluginDraft("");
  };

  return {
    ...props,
    ...source,
    ...resources,
    ...submission,
    draft,
    updateDraft,
    formDisabled,
    blocker,
    modelPresets,
    effortPresets,
    pluginDraft,
    setPluginDraft,
    addPlugin,
    changeHost,
    changeImage,
  };
}

export type CreateAgentForm = ReturnType<typeof useCreateAgentForm>;
