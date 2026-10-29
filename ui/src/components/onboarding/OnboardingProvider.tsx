import { lazy, Suspense, useEffect, useMemo, useState, type ReactNode } from "react";
import { TOUR_STEPS, tourMode, whatsNewSteps, type TourStep } from "@/lib/onboarding/tour";
import { UI_VERSION } from "@/lib/onboarding/version";
import { readKey, storageKeys, writeKey } from "@/lib/storage";
import { OnboardingContext, type OnboardingCtx } from "./onboardingContext";

// The overlay and the changelog (with CHANGELOG.md itself) load on first open.
const TourOverlay = lazy(() => import("./TourOverlay"));
const ChangelogDialog = lazy(() => import("./ChangelogDialog"));

interface Tour {
  steps: TourStep[];
  whatsNew: boolean;
}

/** The tour that starts by itself for the version the user saw last. */
function firstTour(saved: string): Tour | null {
  const mode = tourMode(saved, UI_VERSION, TOUR_STEPS);
  if (mode === "full") return { steps: [...TOUR_STEPS], whatsNew: false };
  if (mode === "whats-new") {
    return { steps: whatsNewSteps(TOUR_STEPS, saved, UI_VERSION), whatsNew: true };
  }
  return null;
}

/** Owns the onboarding tour and the changelog: starts the tour on the first run
 *  or after an update that changed a step, and lets the Help menu reopen both. */
export function OnboardingProvider({ children }: { children: ReactNode }) {
  const [saved] = useState(() => readKey(storageKeys.onboarding));
  const [tour, setTour] = useState<Tour | null>(() => firstTour(saved));
  // The version the changelog marks as seen; null while the window is closed.
  const [changelogSince, setChangelogSince] = useState<string | null>(null);

  useEffect(() => {
    // An update without new steps only moves the seen version forward.
    if (saved !== "" && saved !== UI_VERSION && !firstTour(saved)) {
      writeKey(storageKeys.onboarding, UI_VERSION);
    }
  }, [saved]);

  const value = useMemo<OnboardingCtx>(
    () => ({
      startTour: (mode) => {
        if (mode === "full") {
          setTour({ steps: [...TOUR_STEPS], whatsNew: false });
          return;
        }
        const steps = TOUR_STEPS.filter((step) => step.since === UI_VERSION);
        if (steps.length > 0) setTour({ steps, whatsNew: true });
        else setChangelogSince("");
      },
      openChangelog: () => setChangelogSince(""),
    }),
    [],
  );

  const finish = () => {
    // A blocked storage keeps the tour closed until the next reload.
    writeKey(storageKeys.onboarding, UI_VERSION);
    setTour(null);
  };

  return (
    <OnboardingContext.Provider value={value}>
      {children}
      <Suspense fallback={null}>
        {tour && (
          <TourOverlay
            steps={tour.steps}
            whatsNew={tour.whatsNew}
            onClose={finish}
            onChangelog={() => {
              finish();
              setChangelogSince(saved);
            }}
          />
        )}
        {changelogSince !== null && (
          <ChangelogDialog since={changelogSince} onClose={() => setChangelogSince(null)} />
        )}
      </Suspense>
    </OnboardingContext.Provider>
  );
}
