import type { ReactNode } from "react";
import type {
  WorkflowImageDefinition,
  WorkflowImageStatus,
  WorkflowImageManifest,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { effectiveLimits, ownerLabel } from "./workflowImages";

const CELL = "px-3 py-2 align-top";
const HEAD = "bg-muted/50 text-left text-xs text-muted-foreground";
const DEFAULT_TIMEOUT = "60s";

type Transition = NonNullable<WorkflowImageStatus["transitions"]>[number];

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-2">
      <h2 className="font-medium">{title}</h2>
      {children}
    </section>
  );
}

function TableSection({
  title,
  columns,
  children,
}: {
  title: string;
  columns: string[];
  children: ReactNode;
}) {
  return (
    <Section title={title}>
      <div className="overflow-x-auto rounded border">
        <table aria-label={title} className="w-full">
          <thead className={HEAD}>
            <tr>
              {columns.map((label) => (
                <th key={label} className={CELL}>
                  {label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>{children}</tbody>
        </table>
      </div>
    </Section>
  );
}

function None() {
  return <p className="text-muted-foreground">None</p>;
}

function ErrorLine({ text }: { text: string }) {
  return text ? (
    <p role="alert" className="text-destructive">
      {text}
    </p>
  ) : null;
}

export function WorkflowImageHeader({
  manifest,
  bound,
  uploadDisabled,
  onUpload,
  canRemove,
  busy,
  onRemove,
  error,
  removeError,
}: {
  manifest: WorkflowImageManifest;
  bound: { text: string; title?: string };
  uploadDisabled: boolean;
  onUpload: () => void;
  canRemove: boolean;
  busy: boolean;
  onRemove: () => void;
  error: string;
  removeError: string;
}) {
  return (
    <header className="space-y-2">
      <h1 className="text-lg font-medium">
        {manifest.name} {manifest.version}
      </h1>
      <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-xs">
        <dt className="text-muted-foreground">Digest</dt>
        <dd className="break-all font-mono">{manifest.digest}</dd>
        <dt className="text-muted-foreground">Built at</dt>
        <dd>{manifest.built_at}</dd>
        <dt className="text-muted-foreground">Initial status</dt>
        <dd className="font-mono">{manifest.definition.initial_status}</dd>
        <dt className="text-muted-foreground">Bound to</dt>
        <dd data-testid="bound-to" title={bound.title}>
          {bound.text}
        </dd>
      </dl>
      <div className="flex gap-2">
        <Button size="sm" variant="outline" disabled={uploadDisabled} onClick={onUpload}>
          Upload to servers
        </Button>
        {canRemove && (
          <Button size="sm" variant="destructive" disabled={busy} onClick={onRemove}>
            Remove tag
          </Button>
        )}
      </div>
      <ErrorLine text={error} />
      <ErrorLine text={removeError} />
    </header>
  );
}

function TransitionItem({ transition }: { transition: Transition }) {
  return (
    <li>
      <span className="font-mono">
        {transition.on} → {transition.to}
      </span>
      {transition.requires?.length ? (
        <div className="text-xs text-muted-foreground">
          requires {transition.requires.join(", ")}
        </div>
      ) : null}
      {(transition.checks ?? []).map((check, index) => (
        <div key={index} className="font-mono text-xs text-muted-foreground">
          check {check.script} as {check.run_as || "queue"}, {check.timeout || DEFAULT_TIMEOUT}
        </div>
      ))}
    </li>
  );
}

function StatusRow({
  status,
  initialStatus,
}: {
  status: WorkflowImageStatus;
  initialStatus: string;
}) {
  return (
    <tr className="border-t">
      <td className={CELL}>
        <span className="font-mono">{status.id}</span>
        {[
          status.id === initialStatus && "initial",
          status.terminal && "terminal",
          status.cancelled && "cancelled",
        ]
          .filter(Boolean)
          .map((label) => (
            <span key={String(label)} className="ml-1 rounded bg-muted px-1 text-xs">
              {label}
            </span>
          ))}
      </td>
      <td className={`${CELL} font-mono`}>{ownerLabel(status)}</td>
      <td className={`${CELL} font-mono text-xs`}>{status.instructions ?? "—"}</td>
      <td className={CELL}>
        <ul className="space-y-1">
          {(status.transitions ?? []).map((transition) => (
            <TransitionItem key={transition.on} transition={transition} />
          ))}
        </ul>
      </td>
      <td className={`${CELL} font-mono text-xs`}>
        {status.watch
          ? `${status.watch.script} every ${status.watch.every}, timeout ${status.watch.timeout || DEFAULT_TIMEOUT}`
          : "—"}
      </td>
    </tr>
  );
}

export function StatusesTable({ definition }: { definition: WorkflowImageDefinition }) {
  return (
    <TableSection
      title="Statuses"
      columns={["Status", "Owner", "Instructions", "Outcomes", "Watch"]}
    >
      {definition.statuses.map((status) => (
        <StatusRow key={status.id} status={status} initialStatus={definition.initial_status} />
      ))}
    </TableSection>
  );
}

export function LimitsTable({ definition }: { definition: WorkflowImageDefinition }) {
  return (
    <TableSection
      title="Limits"
      columns={[
        "Scope",
        "idle_iterations",
        "rejected_requests",
        "script_failures",
        "unavailable_grace",
      ]}
    >
      {effectiveLimits(definition).map(({ scope, values }) => (
        <tr key={scope} className="border-t">
          <td className={`${CELL} font-mono`}>{scope}</td>
          {values.map((limit) => (
            <td key={limit.key} className={CELL}>
              {limit.value} <span className="text-xs text-muted-foreground">({limit.source})</span>
            </td>
          ))}
        </tr>
      ))}
    </TableSection>
  );
}

export function ArtifactsList({ definition }: { definition: WorkflowImageDefinition }) {
  return (
    <Section title="Artifacts">
      {definition.artifacts?.length ? (
        <ul className="space-y-1">
          {definition.artifacts.map((artifact) => (
            <li key={artifact.name}>
              <span className="font-mono">{artifact.name}</span>
              {artifact.description && (
                <span className="text-muted-foreground"> — {artifact.description}</span>
              )}
            </li>
          ))}
        </ul>
      ) : (
        <None />
      )}
    </Section>
  );
}

export function SecretsList({ definition }: { definition: WorkflowImageDefinition }) {
  return (
    <Section title="Requires secrets">
      {definition.requires_secrets?.length ? (
        <ul className="font-mono">
          {definition.requires_secrets.map((secret) => (
            <li key={secret}>{secret}</li>
          ))}
        </ul>
      ) : (
        <None />
      )}
    </Section>
  );
}

export function EnvList({ definition }: { definition: WorkflowImageDefinition }) {
  return (
    <Section title="Env">
      {Object.keys(definition.env ?? {}).length ? (
        <ul className="font-mono">
          {Object.entries(definition.env!).map(([key, value]) => (
            <li key={key}>
              {key}={value}
            </li>
          ))}
        </ul>
      ) : (
        <None />
      )}
    </Section>
  );
}

export function FilesTable({ files }: { files: WorkflowImageManifest["files"] }) {
  return (
    <TableSection title="Files" columns={["Path", "Size", "Executable"]}>
      {files.map((file) => (
        <tr key={file.path} className="border-t">
          <td className={`${CELL} font-mono`}>
            <span title={file.sha256}>{file.path}</span>
          </td>
          <td className={CELL}>{file.size} B</td>
          <td className={CELL}>{file.executable ? "yes" : "no"}</td>
        </tr>
      ))}
    </TableSection>
  );
}
