import type { OnboardingStatus } from "../api-types.generated";
import type { Transport } from "./client";

export type {
  OnboardingDefaultStore,
  OnboardingHarness,
  OnboardingStatus,
} from "../api-types.generated";

export function onboardingApi(t: Transport) {
  return {
    /** The first-run state; refresh looks up the coding tool CLIs again. */
    getStatus: (refresh = false) =>
      t.get<OnboardingStatus>(`/api/onboarding${refresh ? "?refresh=true" : ""}`),
    /** Registers the default Store and builds the starter image again when missing. */
    retry: () => t.post<OnboardingStatus>("/api/onboarding/retry"),
  };
}
