import { useEffect, useMemo, useState } from "react";
import { useOptionalDaemons } from "@/components/daemonContext";
import { resolveDaemon, type Daemon } from "@/lib/daemons";

export function useTransferDaemons() {
  const daemonContext = useOptionalDaemons();
  const registryDaemons = useMemo(() => daemonContext?.daemons ?? [], [daemonContext?.daemons]);
  const registryGeneration = useMemo(
    () =>
      JSON.stringify(
        registryDaemons.map((daemon) => ({
          id: daemon.id,
          label: daemon.label,
          baseURL: daemon.baseURL,
          state: daemon.state,
        })),
      ),
    [registryDaemons],
  );
  const [resolvedTransferDaemons, setResolvedTransferDaemons] = useState<{
    generation: string;
    daemons: Daemon[];
  }>({ generation: "", daemons: [] });

  useEffect(() => {
    let alive = true;
    void Promise.allSettled(registryDaemons.map((daemon) => resolveDaemon(daemon.id))).then(
      (results) => {
        if (!alive) return;
        setResolvedTransferDaemons({
          generation: registryGeneration,
          daemons: results.flatMap((result, index) =>
            result.status === "fulfilled" && result.value
              ? [{ ...registryDaemons[index], ...result.value }]
              : [],
          ),
        });
      },
    );
    return () => {
      alive = false;
    };
  }, [registryDaemons, registryGeneration]);

  return {
    daemons: resolvedTransferDaemons.daemons,
    ready: resolvedTransferDaemons.generation === registryGeneration,
  };
}
