import { createContext, useContext } from "react";

export interface OnboardingCtx {
  /** The whole tour, or only the steps of the running release. False when the
   *  tour did not open because a daemon error banner is on screen. */
  startTour: (mode: "full" | "whats-new") => boolean;
  openChangelog: () => void;
}

export const OnboardingContext = createContext<OnboardingCtx | null>(null);

/** Null outside the provider, so screens rendered alone in tests still work. */
export function useOnboarding(): OnboardingCtx | null {
  return useContext(OnboardingContext);
}
