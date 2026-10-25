import { type ReactNode } from "react";
import { useImageContext } from "@/components/imageContext";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { visibleImageSections } from "./imageOverviewSections";

// Read-only presentation of an image manifest. Every section renders only when
// it has content, so a lean image (no plugins/env) stays uncluttered.

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

export default function ImageOverview() {
  const { manifest } = useImageContext();
  if (!manifest) return <p className="text-sm text-muted-foreground">Loading…</p>;

  return (
    <div className="grid gap-4">
      {visibleImageSections(manifest).map((section) => (
        <Section key={section.title} title={section.title}>
          {section.render(manifest)}
        </Section>
      ))}
    </div>
  );
}
