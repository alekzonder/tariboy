import { createContext, useContext } from "react";

export interface OnboardingCtx {
  /** The whole tour, or only the steps of the running release. */
  startTour: (mode: "full" | "whats-new") => void;
  openChangelog: () => void;
}

export const OnboardingContext = createContext<OnboardingCtx | null>(null);

/** Null outside the provider, so screens rendered alone in tests still work. */
export function useOnboarding(): OnboardingCtx | null {
  return useContext(OnboardingContext);
}
