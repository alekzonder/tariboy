import type { Meta, StoryObj } from "@storybook/react-vite";
import { roles, useTokenValue } from "@/storybook/tokens";

// Every color role of styles/tokens.css, in the theme of the toolbar. A role
// that no group below names goes to "Other", so a new token is never hidden.
const meta = {
  title: "Foundations/Colors",
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj;

const GROUPS: { title: string; note: string; match: RegExp }[] = [
  {
    title: "Surfaces",
    note: "The window is metal; content sits on white islands. One surface, one utility: bg-card, bg-muted, bg-accent.",
    match:
      /^--(background-base|card|popover|secondary|muted|accent|sidebar|sidebar-accent|terminal)$/,
  },
  {
    title: "Text",
    note: "text-foreground for content, text-muted-foreground for everything secondary.",
    match: /^--(foreground|.*-foreground)$/,
  },
  {
    title: "Lines and focus",
    note: "border-border, border-input, ring-ring/50.",
    match: /^--(border|input|ring|sidebar-border)$/,
  },
  {
    title: "Tones",
    note: "One accent, one red, and the two notices.",
    match: /^--(primary|destructive|warning|success)$/,
  },
  {
    title: "Status",
    note: "A fill is only for what is alive and what needs a person. A failed status is --destructive.",
    match: /^--status-/,
  },
  { title: "Scrims", note: "What dims the page under a dialog.", match: /^--scrim/ },
];
const SKIP = /^--(hue|tint)-|^--(background|raise|lift)$/;

function Swatch({ name }: { name: string }) {
  const value = useTokenValue(name);
  return (
    <div className="flex min-w-0 items-center gap-3">
      <span
        className="size-10 shrink-0 rounded-control border border-border"
        style={{ background: `var(${name})` }}
      />
      <span className="flex min-w-0 flex-col">
        <span className="truncate font-mono text-ui">{name}</span>
        <span className="truncate font-mono text-caption text-muted-foreground">{value}</span>
      </span>
    </div>
  );
}

function Group({ title, note, names }: { title: string; note: string; names: string[] }) {
  if (names.length === 0) return null;
  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-title font-semibold">{title}</h2>
        <p className="text-body text-muted-foreground">{note}</p>
      </div>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-3">
        {names.map((name) => (
          <Swatch key={name} name={name} />
        ))}
      </div>
    </section>
  );
}

export const Roles: Story = {
  render: () => {
    const all = roles().filter((name) => !SKIP.test(name));
    const grouped = GROUPS.map((group) => ({
      ...group,
      names: all.filter((name) => group.match.test(name)),
    }));
    const other = all.filter((name) => !GROUPS.some((group) => group.match.test(name)));
    return (
      <div className="flex flex-col gap-8 bg-card p-6 text-foreground">
        {grouped.map((group) => (
          <Group key={group.title} {...group} />
        ))}
        <Group title="Other" note="Roles that no group above names." names={other} />
      </div>
    );
  },
};

// The class names are written out: Tailwind finds a class only as a whole word
// in the source.
const TINTS = [
  {
    tone: "primary",
    text: "text-primary",
    soft: "bg-primary-soft",
    mid: "bg-primary-mid",
    line: "border-primary-line",
  },
  {
    tone: "destructive",
    text: "text-destructive",
    soft: "bg-destructive-soft",
    mid: "bg-destructive-mid",
    line: "border-destructive-line",
  },
  {
    tone: "status-running",
    text: "text-status-running",
    soft: "bg-status-running-soft",
    mid: "bg-status-running-mid",
    line: "border-status-running-line",
  },
  {
    tone: "warning",
    text: "text-warning",
    soft: "bg-warning-soft",
    mid: "bg-warning-mid",
    line: "border-warning-line",
  },
  {
    tone: "success",
    text: "text-success",
    soft: "bg-success-soft",
    mid: "bg-success-mid",
    line: "border-success-line",
  },
];

/** The three tints of each tone: a fill, the fill hovered or a focus ring, a border. */
export const Tints: Story = {
  render: () => (
    <div className="flex flex-col gap-4 bg-card p-6 text-foreground">
      <p className="max-w-2xl text-body text-muted-foreground">
        A tone is never mixed by hand. Use the tint: -soft for a fill, -mid for that fill hovered or
        for a focus ring, -line for a border. The strength of the three is --tint-soft, --tint-mid
        and --tint-line, and it is higher in the dark theme.
      </p>
      {TINTS.map((tint) => (
        <div key={tint.tone} className="grid grid-cols-[9rem_repeat(3,minmax(0,1fr))] gap-3">
          <span className={`self-center font-mono text-ui ${tint.text}`}>{tint.tone}</span>
          <span className={`rounded-control px-3 py-2 font-mono text-ui ${tint.soft} ${tint.text}`}>
            {tint.soft}
          </span>
          <span className={`rounded-control px-3 py-2 font-mono text-ui ${tint.mid} ${tint.text}`}>
            {tint.mid}
          </span>
          <span
            className={`rounded-control border px-3 py-2 font-mono text-ui ${tint.line} ${tint.text}`}
          >
            {tint.line}
          </span>
        </div>
      ))}
      <div className="grid grid-cols-[9rem_repeat(3,minmax(0,1fr))] gap-3">
        <span className="self-center font-mono text-ui text-muted-foreground">neutral</span>
        <span className="rounded-control bg-subtle px-3 py-2 font-mono text-ui">bg-subtle</span>
        <span className="rounded-control bg-muted px-3 py-2 font-mono text-ui">bg-muted</span>
        <span className="rounded-control bg-accent px-3 py-2 font-mono text-ui">bg-accent</span>
      </div>
    </div>
  ),
};
