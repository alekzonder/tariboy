import { useCallback, useEffect, useState } from "react";
import type { ApiClient, OnboardingStatus } from "@/lib/api";
import { onboardingSettled } from "@/lib/onboarding/firstRun";

const POLL_MS = 5000;

/** The first-run status of one host, polled while a row can still change by
 *  itself. A daemon without the route (older than the onboarding commands)
 *  and a failed read both give a null status: the checklist is then hidden. */
export function useOnboardingStatus(api: ApiClient | null) {
  const [status, setStatus] = useState<OnboardingStatus | null>(null);
  const [loaded, setLoaded] = useState(false);
  // Bumped by Retry so a settled status starts polling again.
  const [round, setRound] = useState(0);

  useEffect(() => {
    if (!api) return;
    let alive = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const tick = async () => {
      let next: OnboardingStatus | null = null;
      try {
        next = await api.onboarding.getStatus();
      } catch {
        next = null;
      }
      if (!alive) return;
      setStatus(next);
      setLoaded(true);
      if (next && !onboardingSettled(next)) timer = setTimeout(() => void tick(), POLL_MS);
    };
    // After Retry the status it returned stays until the next period.
    if (round === 0) void tick();
    else timer = setTimeout(() => void tick(), POLL_MS);
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [api, round]);

  const retry = useCallback(async () => {
    if (!api) return;
    try {
      setStatus(await api.onboarding.retry());
    } finally {
      setRound((value) => value + 1);
    }
  }, [api]);

  return { status: api ? status : null, loaded: api ? loaded : false, retry };
}
