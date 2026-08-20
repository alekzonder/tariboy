import { type ReactNode } from "react";
import { useImageContext } from "@/components/ImageLayout";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

// Read-only presentation of an image manifest. Every section renders only when
// it has content, so a lean image (no plugins/evals/env) stays uncluttered.

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{title}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">{children}</CardContent>
    </Card>
  );
}

function Row({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="flex gap-2">
      <span className="w-32 shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 flex-1 break-words font-mono text-xs">{value}</span>
    </div>
  );
}

function Chips({ items }: { items: string[] }) {
  return (
    <div className="flex flex-wrap gap-1">
      {items.map((s) => (
        <span key={s} className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">{s}</span>
      ))}
    </div>
  );
}

export default function ImageOverview() {
  const { manifest } = useImageContext();
  if (!manifest) return <p className="text-sm text-muted-foreground">Loading…</p>;

  const { harness, policy, plugins, env, evals, parents, requires_secrets, layers } = manifest;
  const envKeys = env ? Object.keys(env) : [];

  return (
    <div className="grid gap-4">
      {manifest.bare && (
        <Section title="Terminal-only">
          <p className="text-muted-foreground">
            This built-in image has no prompt or capabilities. It starts a normal
            interactive harness session and cannot be rebuilt or automated.
          </p>
        </Section>
      )}
      {manifest.schema_version===1&&harness&&<Section title="Harness">
        <Row label="type" value={harness.type} />
        {harness.model && <Row label="model" value={harness.model} />}
        {harness.effort && <Row label="effort" value={harness.effort} />}
        <Row label="interactive" value={String(harness.interactive)} />
      </Section>}

      {(policy?.tools_allow?.length || policy?.tools_deny?.length) ? (
        <Section title="Policy">
          {policy?.tools_allow?.length ? (
            <div className="space-y-1">
              <div className="text-muted-foreground">tools_allow</div>
              <Chips items={policy.tools_allow} />
            </div>
          ) : null}
          {policy?.tools_deny?.length ? (
            <div className="space-y-1">
              <div className="text-muted-foreground">tools_deny</div>
              <Chips items={policy.tools_deny} />
            </div>
          ) : null}
        </Section>
      ) : null}

      {plugins?.length ? (
        <Section title="Plugins">
          {plugins.map((p) => (
            <Row key={p.name} label={p.name} value={p.version ?? "—"} />
          ))}
        </Section>
      ) : null}

      {envKeys.length ? (
        <Section title="Env">
          {envKeys.map((k) => (
            <Row key={k} label={k} value={env![k]} />
          ))}
        </Section>
      ) : null}

      {evals?.length ? (
        <Section title="Evals">
          {evals.map((e) => (
            <div key={e.name} className="space-y-0.5">
              <div className="font-mono text-xs">
                {e.name} <span className="text-muted-foreground">({e.type})</span>
              </div>
              <div className="whitespace-pre-wrap text-xs text-muted-foreground">{e.prompt}</div>
            </div>
          ))}
        </Section>
      ) : null}

      {parents?.length ? (
        <Section title="Parents">
          <Chips items={parents} />
        </Section>
      ) : null}

      {requires_secrets?.length ? (
        <Section title="Requires secrets">
          <Chips items={requires_secrets} />
        </Section>
      ) : null}

      {layers?.length ? (
        <Section title="Layers">
          {layers.map((l) => (
            <Row key={l.name} label={l.name} value={l.sha256.slice(0, 12)} />
          ))}
        </Section>
      ) : null}
    </div>
  );
}
