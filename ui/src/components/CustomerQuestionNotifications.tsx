import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { useDaemons } from "@/components/daemonContext";
import { hasLocalDaemon, onTaskNotificationActivated, showTaskNotification } from "@/lib/desktop";
import { resolveDaemon, type Daemon } from "@/lib/daemons";
import { createApi, letterTaskKey } from "@/lib/api";
import { serverPath } from "@/lib/terminalsHost";
import {
  CustomerQuestionNotificationsContext,
  type CustomerQuestionNotificationsValue,
} from "./customerQuestionNotificationsContext";
import type { FreshQuestion } from "./customerQuestionNotificationModel";
import { HostQuestionWatcher } from "./HostQuestionWatcher";

type HostSnapshot = {
  attention: ReadonlyMap<string, number>;
};

const CUSTOMER_QUESTION_ACTIVATION_TIMEOUT_MS = 1000;

function withTimeout<T>(operation: Promise<T>, timeoutMs: number): Promise<T> {
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(
      () => reject(new Error("customer question notification request timed out")),
      timeoutMs,
    );
    operation.then(
      (value) => {
        clearTimeout(timeout);
        resolve(value);
      },
      (error: unknown) => {
        clearTimeout(timeout);
        reject(error);
      },
    );
  });
}

export function CustomerQuestionNotifications({ children }: { children: ReactNode }) {
  const { daemons } = useDaemons();
  const navigate = useNavigate();
  const refreshers = useRef(new Map<string, () => Promise<void>>());
  const [snapshots, setSnapshots] = useState(new Map<string, HostSnapshot>());
  const hosts = useMemo(
    () => [
      ...(hasLocalDaemon() ? [{ id: "", label: "This daemon (local)" }] : []),
      ...daemons.map((host) => ({
        id: host.id,
        label: host.label,
      })),
    ],
    [daemons],
  );

  const receiveSnapshot = useCallback(
    (
      hostId: string,
      label: string,
      attention: ReadonlyMap<string, number>,
      fresh: FreshQuestion[],
    ) => {
      setSnapshots((current) => {
        const next = new Map(current);
        next.set(hostId, { attention });
        return next;
      });
      for (const { letter, agentName } of fresh) {
        void showTaskNotification({
          host_id: hostId,
          notification_id: letter.id,
          task_key: letterTaskKey(letter),
          server_label: label,
          agent_name: agentName,
        }).catch(() => {
          // A denied/unavailable native notification cannot change the HTTP snapshot.
        });
      }
    },
    [],
  );

  const removeSnapshot = useCallback((hostId: string) => {
    setSnapshots((current) => {
      if (!current.has(hostId)) return current;
      const next = new Map(current);
      next.delete(hostId);
      return next;
    });
    refreshers.current.delete(hostId);
  }, []);

  const registerRefresher = useCallback((hostId: string, refresh: (() => Promise<void>) | null) => {
    if (refresh) refreshers.current.set(hostId, refresh);
    else refreshers.current.delete(hostId);
  }, []);

  const refreshHost = useCallback(async (hostId: string) => {
    await refreshers.current.get(hostId)?.();
  }, []);

  const attention = useMemo(() => {
    const next = new Map<string, number>();
    for (const snapshot of snapshots.values()) {
      for (const [key, count] of snapshot.attention) next.set(key, count);
    }
    return next;
  }, [snapshots]);
  const value = useMemo<CustomerQuestionNotificationsValue>(
    () => ({ attention, refreshHost }),
    [attention, refreshHost],
  );

  useEffect(
    () =>
      onTaskNotificationActivated((activation) => {
        void (async () => {
          let target: Daemon | null | undefined;
          try {
            target =
              activation.host_id === ""
                ? null
                : await withTimeout(
                    resolveDaemon(activation.host_id),
                    CUSTOMER_QUESTION_ACTIVATION_TIMEOUT_MS,
                  );
            if (activation.host_id !== "" && (!target || target.id !== activation.host_id)) return;
            // Each watched host has its own client; the activation names the host.
            await withTimeout(
              createApi(target ?? null).mail.read([activation.notification_id]),
              CUSTOMER_QUESTION_ACTIVATION_TIMEOUT_MS,
            );
          } catch {
            // The route still identifies the original host and task, so the user
            // can answer even when marking the letter read was interrupted.
          } finally {
            navigate(
              `${serverPath(activation.host_id, "tasks")}?task=${encodeURIComponent(activation.task_key)}`,
            );
            void withTimeout(
              refreshHost(activation.host_id),
              CUSTOMER_QUESTION_ACTIVATION_TIMEOUT_MS,
            ).catch(() => {});
          }
        })();
      }),
    [navigate, refreshHost],
  );

  return (
    <CustomerQuestionNotificationsContext.Provider value={value}>
      {hosts.map((host) => (
        <HostQuestionWatcher
          key={host.id || "__local__"}
          hostId={host.id}
          label={host.label}
          onSnapshot={receiveSnapshot}
          onRemove={removeSnapshot}
          registerRefresher={registerRefresher}
        />
      ))}
      {children}
    </CustomerQuestionNotificationsContext.Provider>
  );
}
