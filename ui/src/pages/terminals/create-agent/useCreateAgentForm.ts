import { useEffect, useState } from "react";
import { useHarnessCatalog } from "@/hooks/useHarnessCatalog";
import { useOnboardingStatus } from "@/hooks/useOnboardingStatus";
import type { OnboardingStatus } from "@/lib/api";
import { runtimePresetGroups } from "@/lib/runtimePresets";
import { harnessLabel, type ImageChoice } from "@/lib/onboarding/firstRun";
import { newAgentDraft, type AgentCreateDraft } from "../agentCreateDraft";
import type { CreateAgentDialogProps } from "../CreateAgentDialog";
import { createBlocker } from "./createBlocker";
import { useCloneAgentSource } from "./useCloneAgentSource";
import { useCreateAgentResources } from "./useCreateAgentResources";
import { useCreateAgentSubmit } from "./useCreateAgentSubmit";
import { useFirstRunDefaults } from "./useFirstRunDefaults";

type Resources = ReturnType<typeof useCreateAgentResources>;

/** The chosen coding tool's label when the host reports it as not installed. */
function missingToolLabel(status: OnboardingStatus | null, harness: string): string | null {
  const tool = status?.harnesses.find((entry) => entry.type === harness);
  return tool && !tool.found ? harnessLabel(tool.type) : null;
}

/** The selected host's label and the agent names already used on it. */
function hostFacts(hosts: CreateAgentDialogProps["hosts"], host: string) {
  const entry = hosts.find((candidate) => candidate.id === host);
  return { label: entry?.label || host || "local", taken: entry?.agents ?? [] };
}

/** The plugin list with a typed plugin added once. */
function withPlugin(plugins: string[], typed: string): string[] {
  const plugin = typed.trim();
  return plugin && !plugins.includes(plugin) ? [...plugins, plugin] : plugins;
}

/** What still blocks Start agent, from the form's current state. */
function formBlocker({
  sourceState,
  hostLabel,
  resources,
  draft,
  choice,
  missingTool,
}: {
  sourceState: ReturnType<typeof useCloneAgentSource>["sourceState"];
  hostLabel: string;
  resources: Resources;
  draft: AgentCreateDraft;
  choice: ImageChoice | null;
  missingTool: string | null;
}) {
  const current = resources.manifestKey === resources.expectedManifestKey;
  return createBlocker({
    sourceState,
    hostLabel,
    targetIsCurrent: resources.targetIsCurrent,
    imagesError: resources.imagesError,
    image: draft.image,
    imageAvailable: resources.targetImageAvailable,
    manifestError: current ? resources.manifestError : "",
    manifestIsCurrent: resources.manifestIsCurrent,
    imageWaiting: choice?.waiting ?? null,
    folderMissing: !draft.managedFolder && !draft.cwd.trim(),
    missingTool,
  });
}

/** Model and effort suggestions for the selected coding tool on the selected host. */
function useRuntimePresets(draft: AgentCreateDraft, resources: Resources, ready: boolean) {
  // An unresolved host asks nothing: a null target would address the local
  // daemon instead of the selected one.
  const catalog = useHarnessCatalog(resources.api, ready ? draft.harness : "");
  return {
    modelPresets: runtimePresetGroups({
      harness: draft.harness,
      field: "models",
      catalog,
      extras: [resources.defaultModel, draft.model],
    }),
    effortPresets: runtimePresetGroups({
      harness: draft.harness,
      field: "efforts",
      catalog,
      model: draft.model,
      extras: [resources.defaultEffort, draft.effort],
    }),
  };
}

export function useCreateAgentForm(props: CreateAgentDialogProps) {
  const { imageRef = "", hosts } = props;
  const [draft, setDraft] = useState<AgentCreateDraft>(() => newAgentDraft(imageRef));
  const [pluginDraft, setPluginDraft] = useState("");
  const updateDraft = <K extends keyof AgentCreateDraft>(key: K, value: AgentCreateDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };
  const source = useCloneAgentSource(props.cloneSource, setDraft);
  const resources = useCreateAgentResources(props, draft, setDraft, updateDraft);
  const { host, targetIsCurrent, reloadImages } = resources;
  const ready = targetIsCurrent && resources.targetIsUsable;
  const onboarding = useOnboardingStatus(ready ? resources.api : null);
  const starterState = onboarding.status?.starter_image.state ?? "";
  // A built starter image appears in the image list.
  useEffect(() => {
    if (starterState) reloadImages();
  }, [starterState, reloadImages]);
  const hostEntry = hostFacts(hosts, host);
  const defaults = useFirstRunDefaults({
    draft,
    setDraft,
    images: resources.images,
    status: onboarding.status,
    targetIsCurrent,
    manifestIsCurrent: resources.manifestIsCurrent,
    defaultHarness: resources.defaultHarness,
    takenNames: hostEntry.taken,
    fixedImage: imageRef !== "" || props.cloneSource !== undefined,
    clone: props.cloneSource !== undefined,
  });
  const presets = useRuntimePresets(draft, resources, ready);
  // The seeded bare:latest does not always carry the bare flag, so its name counts too.
  const plainTerminal = resources.bare || defaults.choice?.name === "bare";
  const blocker = formBlocker({
    sourceState: source.sourceState,
    hostLabel: hostEntry.label,
    resources,
    draft,
    choice: defaults.choice,
    // A plain terminal runs no coding tool.
    missingTool: plainTerminal ? null : missingToolLabel(onboarding.status, draft.harness),
  });

  const submission = useCreateAgentSubmit(draft, resources, blocker, props);
  const formDisabled =
    submission.busy || submission.created !== null || source.sourceState === "loading";
  const changeHost = (value: string) => {
    resources.changeHost(value);
    submission.setSubmitError("");
  };
  const changeImage = (value: string) => {
    defaults.touchImage();
    resources.changeImage(value);
    submission.setSubmitError("");
  };
  const changeHarness = (value: string) => {
    defaults.touchHarness();
    updateDraft("harness", value);
  };
  const changeName = (value: string) => {
    defaults.touchName();
    updateDraft("name", value);
  };
  const addPlugin = () => {
    updateDraft("plugins", withPlugin(draft.plugins, pluginDraft));
    setPluginDraft("");
  };

  return {
    ...props,
    ...source,
    ...resources,
    ...submission,
    ...presets,
    draft,
    updateDraft,
    formDisabled,
    blocker,
    pluginDraft,
    setPluginDraft,
    addPlugin,
    changeHost,
    changeImage,
    changeHarness,
    changeName,
    onboarding,
    plainTerminal,
    imageChoices: defaults.choices,
    imageChoice: defaults.choice,
  };
}

export type CreateAgentForm = ReturnType<typeof useCreateAgentForm>;
