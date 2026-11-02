/** "—": the value is not known (never zero). The hint and the screen reader
 *  give the reason. */
export function Unknown({ reason }: { reason: string }) {
  return (
    <span title={reason} className="text-muted-foreground">
      <span aria-hidden="true">—</span>
      <span className="sr-only">not known: {reason}</span>
    </span>
  );
}

/** `format(value)`, or Unknown when the value is null or missing. */
export function Known<T>({
  value,
  format,
  reason,
}: {
  value: T | null | undefined;
  format: (value: T) => string;
  reason: string;
}) {
  if (value === null || value === undefined) return <Unknown reason={reason} />;
  return <>{format(value)}</>;
}
