// One labelled metric of a Usage summary. Render it inside a <dl>.
export default function UsageSummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border p-3">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 font-mono text-lg">{value}</dd>
    </div>
  );
}
