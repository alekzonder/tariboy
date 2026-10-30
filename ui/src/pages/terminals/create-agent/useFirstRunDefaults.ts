import { useEffect, useRef, type Dispatch, type SetStateAction } from "react";
import type { ImageSummary, OnboardingStatus } from "@/lib/api";
import {
  imageChoices,
  pickHarness,
  starterWaitReason,
  suggestAgentName,
} from "@/lib/onboarding/firstRun";
import type { AgentCreateDraft } from "../agentCreateDraft";

const imageName = (ref: string) => ref.split(":")[0] ?? "";

/** The choices the New agent form makes for the user until the user makes
 *  them: the first image, the detected coding tool, and a unique name. */
export function useFirstRunDefaults({
  draft,
  setDraft,
  images,
  status,
  targetIsCurrent,
  manifestIsCurrent,
  defaultHarness,
  takenNames,
  fixedImage,
  clone,
}: {
  draft: AgentCreateDraft;
  setDraft: Dispatch<SetStateAction<AgentCreateDraft>>;
  images: ImageSummary[];
  status: OnboardingStatus | null;
  targetIsCurrent: boolean;
  manifestIsCurrent: boolean;
  defaultHarness: string;
  takenNames: string[];
  /** The image came from the entry point (Run Agent, clone): keep it. */
  fixedImage: boolean;
  clone: boolean;
}) {
  const touched = useRef({ image: fixedImage, harness: clone, name: false });
  const waiting = starterWaitReason(status);
  const choices = imageChoices(images, waiting);
  const choice = choices.find((entry) => entry.name === imageName(draft.image)) ?? null;
  const firstRef = targetIsCurrent ? (choices[0]?.ref ?? "") : "";

  useEffect(() => {
    if (touched.current.image || !firstRef || draft.image === firstRef) return;
    setDraft((current) => ({ ...current, image: firstRef }));
  }, [firstRef, draft.image, setDraft]);

  const harnesses = status?.harnesses ?? null;
  const pickedHarness = manifestIsCurrent ? pickHarness(defaultHarness, harnesses) : "";
  useEffect(() => {
    if (touched.current.harness || !pickedHarness) return;
    setDraft((current) =>
      current.harness === pickedHarness ? current : { ...current, harness: pickedHarness },
    );
  }, [pickedHarness, setDraft]);

  const suggestedName = suggestAgentName(
    draft.managedFolder ? "" : draft.cwd,
    imageName(draft.image),
    takenNames,
  );
  useEffect(() => {
    if (touched.current.name) return;
    setDraft((current) =>
      current.name === suggestedName ? current : { ...current, name: suggestedName },
    );
  }, [suggestedName, setDraft]);

  return {
    choices,
    choice,
    touchImage: () => {
      touched.current.image = true;
    },
    touchHarness: () => {
      touched.current.harness = true;
    },
    touchName: () => {
      touched.current.name = true;
    },
  };
}
