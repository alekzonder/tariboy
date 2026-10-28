import { useCallback, useEffect, useState } from "react";
import { useApi } from "@/components/apiContext";
import type { QueueSecretInfo } from "@/lib/api";
import { COUNT, EMPTY, LABEL, QUIET_ACTION } from "./panelStyles";
import KeyValueForm from "./KeyValueForm";
import type { KeyHint } from "./queueSettingsModel";
import { formatTaskTime } from "./taskTime";
import { useWorkflowConfirm } from "./WorkflowConfirm";
import { errorText } from "@/lib/errors";
import { keyProblem } from "./workflowShared";

/** The required secrets the global env holds, and whether a queue secret overrides each. */
function GlobalEnvList({
  globalEnv,
  secrets,
}: {
  globalEnv: string[];
  secrets: QueueSecretInfo[];
}) {
  if (globalEnv.length === 0) return null;
  return (
    <ul
      aria-label="Global env"
      className="flex min-w-0 flex-col gap-1 text-ui text-muted-foreground"
    >
      {globalEnv.map((name) => (
        <li key={name}>
          <span className="font-mono">{name}</span> present in global env
          {secrets.some((secret) => secret.key === name) && ", overridden by the queue secret"}
        </li>
      ))}
    </ul>
  );
}

function SecretList({
  secrets,
  loaded,
  busy,
  onRemove,
}: {
  secrets: QueueSecretInfo[];
  loaded: boolean;
  busy: boolean;
  onRemove: (name: string) => void;
}) {
  if (secrets.length === 0) return loaded && <span className={EMPTY}>No secrets.</span>;
  return (
    <ul className="flex min-w-0 flex-col gap-1">
      {secrets.map((secret) => (
        <li key={secret.key} className="flex min-w-0 items-center gap-2 text-ui">
          <span className="min-w-0 flex-1 truncate font-mono">{secret.key}</span>
          <span className="text-muted-foreground">{formatTaskTime(secret.updated_at)}</span>
          <button
            type="button"
            className={QUIET_ACTION}
            aria-label={`Remove ${secret.key}`}
            disabled={busy}
            onClick={() => onRemove(secret.key)}
          >
            Remove
          </button>
        </li>
      ))}
    </ul>
  );
}

/**
 * A queue's secrets: keys with their time, Set and Remove, and the required
 * secrets the global env holds, which a queue secret of that name overrides. A value is
 * write-only: it lives in the password input until submit, then is cleared,
 * and nothing here ever reads or renders one.
 */
export default function QueueSecrets({
  queue,
  missing = [],
  missingMessage = "",
  revision = 0,
  envKeys = [],
  hints = [],
  onKeys,
  onChanged,
}: {
  queue: string;
  /** Names a failed bind reported without a value. */
  missing?: string[];
  missingMessage?: string;
  /** A change reloads the list: the Env block moved a key. */
  revision?: number;
  /** Keys of the queue's plain env vars: one also answers a needed name, and setting one here moves it. */
  envKeys?: string[];
  /** The secret names the bound workflow requires. */
  hints?: KeyHint[];
  onKeys?: (keys: string[]) => void;
  /** Told of a change instead of reloading only this list. */
  onChanged?: () => void;
}) {
  const api = useApi();
  const [secrets, setSecrets] = useState<QueueSecretInfo[]>([]);
  const [globalEnv, setGlobalEnv] = useState<string[]>([]);
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const [error, setError] = useState("");
  // Until a load succeeds the list is unknown, so "No secrets." would be a guess.
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const { confirm, dialog } = useWorkflowConfirm();

  const load = useCallback(async () => {
    try {
      const list = await api.workflows.listQueueSecrets(queue);
      setSecrets(list.secrets);
      setGlobalEnv(list.global_env);
      onKeys?.(list.secrets.map((secret) => secret.key));
      setLoaded(true);
    } catch (err) {
      setError(errorText(err));
    }
  }, [queue, api, onKeys]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load, revision]);
  const reload = onChanged ?? load;

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    const name = key.trim();
    const problem = keyProblem(name);
    if (problem) {
      setError(problem);
      return;
    }
    const secret = value;
    const store = async () => {
      setValue("");
      setError("");
      setBusy(true);
      try {
        await api.workflows.setQueueSecret(queue, name, secret);
        setKey("");
        await reload();
      } catch (err) {
        setError(errorText(err));
      } finally {
        setBusy(false);
      }
    };
    if (!envKeys.includes(name)) {
      await store();
      return;
    }
    confirm({
      title: `Move ${name} to secrets?`,
      description: "The env var is removed and the key holds this secret value instead.",
      action: "Move to secrets",
      run: () => void store(),
    });
  };

  const remove = (name: string) =>
    confirm({
      title: `Remove secret ${name}?`,
      description:
        "Scripts that read it will fail until it is set again. The value cannot be recovered.",
      action: "Remove secret",
      run: () => {
        setError("");
        setBusy(true);
        api.workflows
          .removeQueueSecret(queue, name)
          .then(reload, (err) => setError(errorText(err)))
          .finally(() => setBusy(false));
      },
    });
  const stillMissing = missing.filter(
    (name) => !envKeys.includes(name) && !secrets.some((secret) => secret.key === name),
  );

  return (
    <section className="flex min-w-0 flex-col gap-1.5" aria-label={`Secrets ${queue}`}>
      <div className="flex items-center gap-1.5">
        <span className={LABEL}>Secrets</span>
        <span className={COUNT}>{secrets.length}</span>
      </div>
      {stillMissing.length > 0 && (
        <div role="alert" aria-label="Secrets needed" className="text-ui text-destructive">
          <p>{missingMessage || "These secrets have no value:"}</p>
          <p className="font-mono">{stillMissing.join(", ")}</p>
        </div>
      )}
      <GlobalEnvList globalEnv={globalEnv} secrets={secrets} />
      <SecretList secrets={secrets} loaded={loaded} busy={busy} onRemove={(name) => remove(name)} />
      <KeyValueForm
        label="Secret"
        block="Secrets"
        secret
        keyName={key}
        value={value}
        busy={busy}
        hints={hints}
        taken={[...secrets.map((secret) => secret.key), ...envKeys]}
        onKey={setKey}
        onValue={setValue}
        onSubmit={(event) => void submit(event)}
      />
      {error && (
        <p role="alert" className="text-ui text-destructive">
          {error}
        </p>
      )}
      {dialog}
    </section>
  );
}
