import type { ReactNode } from "react";
import type { Operation } from "@/lib/api";
import { formatDuration, formatTime, kindLabel } from "./operationFormat";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </>
  );
}

const mono = "font-mono text-xs break-all";

/** What a finished build produced: one line per image, or the workflow image. */
function OperationResult({ operation }: { operation: Operation }) {
  const image = operation.image_result;
  const workflow = operation.workflow_result;
  if (image) {
    const built = image.images?.length
      ? image.images
      : image.name
        ? [{ name: image.name, tag: image.tag ?? "", digest: image.digest ?? "" }]
        : [];
    return (
      <ul className="space-y-1">
        {built.map((item) => (
          <li key={`${item.name}:${item.tag}`}>
            <span className="font-medium">
              {item.name}:{item.tag}
            </span>{" "}
            <span className={mono}>{item.digest}</span>
          </li>
        ))}
      </ul>
    );
  }
  if (workflow) {
    return (
      <div className="space-y-1">
        <p>
          <span className="font-medium">
            {workflow.name} {workflow.version}
          </span>
          {workflow.created ? "" : " (already built)"}
        </p>
        <p>Tags: {workflow.tags.join(", ") || "none"}</p>
        <p className={mono}>{workflow.digest}</p>
      </div>
    );
  }
  return <span className="text-muted-foreground">—</span>;
}

export function OperationDetails({ operation }: { operation: Operation }) {
  const params = Object.entries(operation.params ?? {});
  const failed = operation.error_code || operation.error_message;
  return (
    <dl className="grid grid-cols-[9rem_1fr] gap-x-4 gap-y-2 rounded border p-4 text-sm max-sm:grid-cols-1">
      <Field label="Kind">{kindLabel(operation.kind)}</Field>
      <Field label="Initiator">{operation.initiator || "—"}</Field>
      <Field label="Started">{formatTime(operation.created_at)}</Field>
      <Field label="Finished">{formatTime(operation.finished_at)}</Field>
      <Field label="Duration">{formatDuration(operation.created_at, operation.finished_at)}</Field>
      <Field label="Parameters">
        {params.length === 0 ? (
          "—"
        ) : (
          <ul className="space-y-0.5">
            {params.map(([key, value]) => (
              <li key={key} className={mono}>
                {key}={value}
              </li>
            ))}
          </ul>
        )}
      </Field>
      {failed ? (
        <Field label="Error">
          <div role="alert" className="text-destructive">
            {operation.error_code && <span className={mono}>{operation.error_code}: </span>}
            {operation.error_message}
          </div>
        </Field>
      ) : (
        <Field label="Result">
          <OperationResult operation={operation} />
        </Field>
      )}
    </dl>
  );
}
