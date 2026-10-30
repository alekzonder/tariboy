import {
  lazy,
  Suspense,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useLocation, useNavigate } from "react-router-dom";
import {
  firstAgentRow,
  TOUR_STEPS,
  tourMode,
  tourSteps,
  whatsNewSteps,
  type TourStep,
} from "@/lib/onboarding/tour";
import { UI_VERSION } from "@/lib/onboarding/version";
import { readKey, storageKeys, writeKey } from "@/lib/storage";
import { OnboardingContext, type OnboardingCtx } from "./onboardingContext";

// The overlay and the changelog (with CHANGELOG.md itself) load on first open.
const TourOverlay = lazy(() => import("./TourOverlay"));
const ChangelogDialog = lazy(() => import("./ChangelogDialog"));

interface Tour {
  steps: TourStep[];
  whatsNew: boolean;
  /** The route the tour started on; closing the tour goes back to it. */
  returnTo: string;
}

/** What's new after an update that changed a step; the full tour never starts by itself. */
function firstTour(saved: string, returnTo: string): Tour | null {
  if (tourMode(saved, UI_VERSION, TOUR_STEPS) !== "whats-new") return null;
  return { steps: whatsNewSteps(TOUR_STEPS, saved, UI_VERSION), whatsNew: true, returnTo };
}

/** A red daemon banner is on screen: the tour would point at a broken app. */
function daemonErrorShown(): boolean {
  return document.querySelector('[data-daemon-banner="error"]') !== null;
}

/** Owns the onboarding tour and the changelog: shows What's new after an update
 *  that changed a step, and lets the Help menu open the tour and the changelog. */
export function OnboardingProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const location = useLocation();
  const here = useRef("");
  useLayoutEffect(() => {
    here.current = location.pathname + location.search;
  });
  const [saved] = useState(() => readKey(storageKeys.onboarding));
  const [tour, setTour] = useState<Tour | null>(() =>
    firstTour(saved, location.pathname + location.search),
  );
  // The version the changelog marks as seen; null while the window is closed.
  const [changelogSince, setChangelogSince] = useState<string | null>(null);

  useEffect(() => {
    // The first run and an update without new steps only record the version as seen.
    if (saved !== UI_VERSION && !firstTour(saved, "")) {
      writeKey(storageKeys.onboarding, UI_VERSION);
    }
  }, [saved]);

  const value = useMemo<OnboardingCtx>(
    () => ({
      startTour: (mode) => {
        const steps =
          mode === "full"
            ? tourSteps(TOUR_STEPS, firstAgentRow() !== null)
            : TOUR_STEPS.filter((step) => step.since === UI_VERSION);
        if (steps.length === 0) {
          setChangelogSince("");
          return true;
        }
        if (daemonErrorShown()) return false;
        setTour({ steps, whatsNew: mode !== "full", returnTo: here.current });
        return true;
      },
      openChangelog: () => setChangelogSince(""),
    }),
    [],
  );

  const finish = () => {
    // A blocked storage keeps the tour closed until the next reload.
    writeKey(storageKeys.onboarding, UI_VERSION);
    if (tour && here.current !== tour.returnTo) navigate(tour.returnTo);
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
