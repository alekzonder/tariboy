import { type ReactNode } from "react";
import type { ImageManifest } from "@/lib/api";
import { Chips, Row, ToolList } from "./imageOverviewParts";

interface ImageSection {
  title: string;
  visible: (manifest: ImageManifest) => boolean;
  render: (manifest: ImageManifest) => ReactNode;
}

const envKeys = (manifest: ImageManifest) => (manifest.env ? Object.keys(manifest.env) : []);

const SECTIONS: ImageSection[] = [
  {
    title: "Terminal-only",
    visible: (manifest) => !!manifest.bare,
    render: () => (
      <p className="text-muted-foreground">
        This built-in image has no prompt or capabilities. It starts a normal interactive harness
        session and cannot be rebuilt or automated.
      </p>
    ),
  },
  {
    title: "Harness",
    visible: (manifest) => manifest.schema_version === 1 && !!manifest.harness,
    render: ({ harness }) => (
      <>
        <Row label="type" value={harness!.type} />
        {harness!.model && <Row label="model" value={harness!.model} />}
        {harness!.effort && <Row label="effort" value={harness!.effort} />}
        <Row label="interactive" value={String(harness!.interactive)} />
      </>
    ),
  },
  {
    title: "Policy",
    visible: ({ policy }) => !!(policy?.tools_allow?.length || policy?.tools_deny?.length),
    render: ({ policy }) => (
      <>
        <ToolList label="tools_allow" items={policy?.tools_allow} />
        <ToolList label="tools_deny" items={policy?.tools_deny} />
      </>
    ),
  },
  {
    title: "Plugins",
    visible: ({ plugins }) => !!plugins?.length,
    render: ({ plugins }) =>
      plugins!.map((p) => <Row key={p.name} label={p.name} value={p.version ?? "—"} />),
  },
  {
    title: "Env",
    visible: (manifest) => envKeys(manifest).length > 0,
    render: (manifest) =>
      envKeys(manifest).map((k) => <Row key={k} label={k} value={manifest.env![k]} />),
  },
  {
    title: "Parents",
    visible: ({ parents }) => !!parents?.length,
    render: ({ parents }) => <Chips items={parents!} />,
  },
  {
    title: "Requires secrets",
    visible: ({ requires_secrets }) => !!requires_secrets?.length,
    render: ({ requires_secrets }) => <Chips items={requires_secrets!} />,
  },
  {
    title: "Layers",
    visible: ({ layers }) => !!layers?.length,
    render: ({ layers }) =>
      layers!.map((l) => <Row key={l.name} label={l.name} value={l.sha256.slice(0, 12)} />),
  },
];

export function visibleImageSections(manifest: ImageManifest) {
  return SECTIONS.filter((section) => section.visible(manifest));
}
