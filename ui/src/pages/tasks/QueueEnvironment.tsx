import { useCallback, useEffect, useState } from "react";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { QueueEnvVar } from "@/lib/api";
import { COUNT, EMPTY, FIELD, LABEL, QUIET_ACTION } from "./panelStyles";
import QueueSecrets from "./QueueSecrets";
import { formatTaskTime } from "./taskTime";
import { useWorkflowConfirm } from "./WorkflowConfirm";
import { errorText } from "@/lib/errors";
import { keyProblem } from "./workflowShared";

/**
 * The Environment tab of a queue: plain env vars and secrets in two blocks.
 * Both reach the scripts of the queue's workflow the same way; a key is one or
 * the other, so a change in either block reloads both.
 */
export default function QueueEnvironment({
  queue,
  missing,
  missingMessage,
}: {
  queue: string;
  /** Names a failed bind reported without a value. */
  missing?: string[];
  missingMessage?: string;
}) {
  const [revision, setRevision] = useState(0);
  const [envKeys, setEnvKeys] = useState<string[]>([]);
  const [secretKeys, setSecretKeys] = useState<string[]>([]);
  const changed = useCallback(() => setRevision((value) => value + 1), []);
  return (
    <div className="flex min-w-0 flex-col gap-5">
      <QueueEnv
        queue={queue}
        revision={revision}
        secretKeys={secretKeys}
        onKeys={setEnvKeys}
        onChanged={changed}
      />
      <QueueSecrets
        queue={queue}
        missing={missing}
        missingMessage={missingMessage}
        revision={revision}
        envKeys={envKeys}
        onKeys={setSecretKeys}
        onChanged={changed}
      />
    </div>
  );
}

/** Plain env vars: keys with their values, Set and Remove. A value is not secret. */
function QueueEnv({
  queue,
  revision,
  secretKeys,
  onKeys,
  onChanged,
}: {
  queue: string;
  revision: number;
  secretKeys: string[];
  onKeys: (keys: string[]) => void;
  onChanged: () => void;
}) {
  const api = useApi();
  const [items, setItems] = useState<QueueEnvVar[]>([]);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [error, setError] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const { confirm, dialog } = useWorkflowConfirm();

  const load = useCallback(async () => {
    try {
      const list = await api.workflows.listQueueEnv(queue);
      setItems(list);
      onKeys(list.map((item) => item.key));
      setLoaded(true);
    } catch (err) {
      setError(errorText(err));
    }
  }, [queue, api, onKeys]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load, revision]);

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    const name = key.trim();
    const problem = keyProblem(name);
    if (problem) {
      setError(problem);
      return;
    }
    const store = async () => {
      setError("");
      setBusy(true);
      try {
        await api.workflows.setQueueEnv(queue, name, value);
        setKey("");
        setValue("");
        onChanged();
      } catch (err) {
        setError(errorText(err));
      } finally {
        setBusy(false);
      }
    };
    if (!secretKeys.includes(name)) {
      void store();
      return;
    }
    confirm({
      title: `Move ${name} to env?`,
      description:
        "The secret is removed and the key holds this plain value instead, which is shown here and in run logs.",
      action: "Move to env",
      run: () => void store(),
    });
  };

  const remove = (name: string) =>
    confirm({
      title: `Remove env var ${name}?`,
      description: "Scripts that read it will not see it until it is set again.",
      action: "Remove env var",
      run: () => {
        setError("");
        setBusy(true);
        api.workflows
          .removeQueueEnv(queue, name)
          .then(onChanged, (err) => setError(errorText(err)))
          .finally(() => setBusy(false));
      },
    });

  return (
    <section className="flex min-w-0 flex-col gap-1.5" aria-label={`Env ${queue}`}>
      <div className="flex items-center gap-1.5">
        <span className={LABEL}>Env</span>
        <span className={COUNT}>{items.length}</span>
      </div>
      {items.length === 0 ? (
        loaded && <span className={EMPTY}>No env vars.</span>
      ) : (
        <ul className="flex min-w-0 flex-col gap-1">
          {items.map((item) => (
            <li key={item.key} className="flex min-w-0 items-center gap-2 text-[12px]">
              <span className="shrink-0 font-mono">{item.key}</span>
              <span className="text-muted-foreground">=</span>
              <span className="min-w-0 flex-1 truncate font-mono" title={item.value}>
                {item.value}
              </span>
              <span className="text-muted-foreground">{formatTaskTime(item.updated_at)}</span>
              <button
                type="button"
                className={QUIET_ACTION}
                aria-label={`Remove ${item.key}`}
                disabled={busy}
                onClick={() => remove(item.key)}
              >
                Remove
              </button>
            </li>
          ))}
        </ul>
      )}
      <form
        className="flex min-w-0 flex-wrap items-center gap-1.5"
        autoComplete="off"
        onSubmit={submit}
      >
        <Input
          aria-label="Env key"
          placeholder="KEY"
          value={key}
          className={`${FIELD} h-[26px] min-w-[100px] flex-1`}
          onChange={(event) => setKey(event.target.value)}
        />
        <Input
          aria-label="Env value"
          placeholder="Value"
          value={value}
          className={`${FIELD} h-[26px] min-w-[100px] flex-1`}
          onChange={(event) => setValue(event.target.value)}
        />
        <Button
          type="submit"
          size="sm"
          className="h-[26px]"
          disabled={busy || !key.trim() || !value}
        >
          Set
        </Button>
      </form>
      {error && (
        <p role="alert" className="text-[12px] text-destructive">
          {error}
        </p>
      )}
      {dialog}
    </section>
  );
}
