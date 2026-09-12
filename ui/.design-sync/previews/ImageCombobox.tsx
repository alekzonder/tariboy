import { useEffect, useRef } from "react";
import { ImageCombobox, Label } from "tariboy-ui";

// ImageCombobox is the required image picker on the agent-create surfaces
// (AgentCreate, CreateAgentDialog, and each row of the group wizard). The value
// is the full `name:tag` ref; the option list is a plain filtered listbox that
// opens on focus. Fixtures follow ImageRow in src/lib/api.ts.

const images = [
  { name: "worker", tag: "v2", bare: false, built_at: "2026-09-11T09:14:00Z", current_agents: ["builder", "reviewer"] },
  { name: "worker", tag: "v1", bare: false, built_at: "2026-08-28T17:02:00Z" },
  { name: "bare", tag: "latest", bare: true, built_at: "2026-09-02T11:40:00Z" },
  { name: "packager", tag: "v3", bare: false, built_at: "2026-09-10T20:31:00Z" },
];

const field = { width: 320, display: "flex", flexDirection: "column", gap: 6 } as const;

export const Selected = () => (
  <div style={field}>
    <Label htmlFor="image-selected">image *</Label>
    <ImageCombobox id="image-selected" images={images} value="worker:v2" onChange={() => {}} />
  </div>
);

export const EmptyRequired = () => (
  <div style={field}>
    <Label htmlFor="image-empty">image *</Label>
    <ImageCombobox id="image-empty" images={images} value="" onChange={() => {}} />
  </div>
);

export const Invalid = () => (
  <div style={field}>
    <Label htmlFor="image-invalid">image *</Label>
    <ImageCombobox id="image-invalid" images={images} value="" onChange={() => {}} invalid />
    <p style={{ margin: 0, fontSize: 12, color: "var(--destructive)" }}>image is required</p>
  </div>
);

// The listbox opens on focus, so this cell focuses the real input on mount
// (nothing is stubbed) to show the filtered option list the picker ships with.
export const OpenList = () => {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.querySelector("input")?.focus({ preventScroll: true });
  }, []);
  return (
    <div ref={ref} style={{ ...field, paddingBottom: 170 }}>
      <Label htmlFor="image-open">image *</Label>
      <ImageCombobox
        id="image-open"
        images={images}
        value="worker:v2"
        onChange={() => {}}
        ariaLabel="image for builder"
      />
    </div>
  );
};

// The group wizard gives each row's picker a distinct accessible name and packs
// it beside the agent name, so the field lives in a tight two-column grid.
export const InGroupWizardRow = () => (
  <div style={{ width: 460, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12, alignItems: "end" }}>
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      <Label htmlFor="wiz-name">name</Label>
      <div
        style={{
          height: 32,
          display: "flex",
          alignItems: "center",
          padding: "0 12px",
          borderRadius: 6,
          border: "1px solid var(--input)",
          font: "13px ui-monospace, SFMono-Regular, Menlo, monospace",
        }}
      >
        packager
      </div>
    </div>
    <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
      <Label htmlFor="wiz-image">image</Label>
      <ImageCombobox
        id="wiz-image"
        images={images}
        value="packager:v3"
        onChange={() => {}}
        ariaLabel="image for packager"
      />
    </div>
  </div>
);
