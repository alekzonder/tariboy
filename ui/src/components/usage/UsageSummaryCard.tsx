// One labelled metric of a Usage summary. Render it inside a <dl>.
export default function UsageSummaryCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-chip border p-3">
      <dt className="text-ui text-muted-foreground">{label}</dt>
      <dd className="mt-1 font-mono text-heading">{value}</dd>
    </div>
  );
}
