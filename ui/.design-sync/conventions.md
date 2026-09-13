## How to build with Tariboy UI

Tariboy UI is the component library of the Tariboy desktop app (Tauri shell, web
UI). It is **shadcn/ui on Tailwind v4**, warm sand neutrals with a single steel
accent, Geist Variable, built for dense operator tooling: agent consoles, task
trees, image and host tables. Designs should read as a control surface, not a
marketing page.

### Setup

No global provider is required — every primitive renders standalone. Three
exceptions, all exported from the bundle:

- `Tooltip` **must** be inside `TooltipProvider`, or it throws.
- Dark mode is a **class**, not a media query: put `class="dark"` on an ancestor
  (`<html>` in the app). `ThemeProvider` does this for you and persists the
  choice; `ThemeToggle` is the titlebar control that drives it.
- `AliasEditor`, `NotesEditor` and `AgentLayout` read `AgentNameContext` /
  `AgentStatusContext` (both exported); everything else takes plain props.
  Router-linked components need a router in scope — `MemoryRouter`, `Routes` and
  `Route` are exported for isolated use, and `AgentLayout` / `ImageLayout` derive
  their identity from `useParams()`, so they need a *matched* route, not just a
  router. `toast` is exported for driving `Toaster`.

### Styling idiom: Tailwind v4 utilities over shadcn tokens

Style with utility classes, exactly as the component sources do. Colors are
**always** semantic tokens, never raw hex or Tailwind color scales — this is what
makes light and dark both work:

| role | utilities |
|---|---|
| surfaces | `bg-background` `bg-card` `bg-popover` `bg-muted` `bg-accent` `bg-secondary` |
| text | `text-foreground` `text-muted-foreground` `text-card-foreground` `text-primary-foreground` |
| emphasis | `bg-primary` + `text-primary-foreground` |
| danger | `text-destructive`, `bg-destructive/10` |
| lines | `border` `border-border` `border-t` `border-b` `border-input` |
| focus | `ring-ring/50` `focus-visible:border-ring` |

The same names exist as CSS custom properties for inline styles and for anything
the utilities don't cover: `--background --foreground --card --popover --primary
--secondary --muted --accent --destructive --border --input --ring --radius`,
each with a `-foreground` partner where it carries text, plus `--sidebar*` and
`--chart-1…5`. 49 in `:root`, 38 redefined under `.dark`.

**Status is a system, not a color choice.** A fill is only for what is alive and
what needs a person; everything else is quiet text: `--status-running` (in
progress) · `--status-failed` (failed, out of budget) · `--primary` (waiting on
the customer) · `--status-queued` / `--status-done` / `--status-stopped`. As
utilities: `text-status-running` with `bg-status-running/13`,
`text-status-failed` with `bg-status-failed/12`, `text-primary` with
`bg-primary/12`. Open, done and cancelled get `text-muted-foreground` and no
fill at all.

**Elevation is two levels, never three**: `--raise` (a selected row, the active
segment of a segmented control) and `--lift` (the content island, menus). The
island itself is `bg-card` + `rounded-[var(--panel-radius)]` +
`shadow-[var(--lift)]` on a `--background` chrome that carries no borders.

**Density and scale.** Body text is `text-sm`; secondary meta is `text-xs`;
headings `text-base`/`text-lg` with `font-medium`/`font-semibold`. Identifiers —
task keys, image refs, digests, paths, terminal output — are `font-mono`.
Controls are short: buttons `h-8` by default (`h-7` sm, `h-6` xs), inputs `h-8`.
Spacing is `gap-1`/`gap-2`/`gap-3` and `p-3`/`p-4`; radius comes from
`--radius: 0.625rem` via `rounded-lg`/`rounded-md`/`rounded-xl`. Icons are
**lucide-react**, 16px at default control size, 14px at sm — never emoji.
The console surfaces are denser than the defaults: rows 30px, buttons 24/26/28px,
pills 17/19/20px, radii 5/6/7/8/9/12/14px. Identifiers, durations and timestamps
are `font-mono` + `tabular-nums` so they line up down a column.

**One caveat that matters.** This bundle ships Tailwind's *compiled* output, so
only utilities the app itself already uses are present (about 960 classes).
Common layout, spacing, color, type and border utilities are all there; anything
the app itself never used is not. If a utility appears to do nothing, express it
as an inline style with `var(--token)` instead. (This file is scanned by Tailwind
like any other source, so naming a class here would compile it into the bundle —
which is why no example of an absent one is given.)

### Where the truth is

`_ds/<folder>/styles.css` and its imports hold every shipped class and token —
read them rather than guessing. Per component, `<Name>.d.ts` is the API contract
and `<Name>.prompt.md` the usage reference with examples.

### Idiomatic example

```jsx
<Card size="sm">
  <CardHeader>
    <CardTitle>worker:v2</CardTitle>
    <CardDescription>Built 2026-09-11 · sha256:9ac71e3f52</CardDescription>
    <CardAction><Badge variant="secondary">Pending</Badge></CardAction>
  </CardHeader>
  <CardContent>
    <div className="flex items-center justify-between gap-2 text-sm">
      <span className="font-mono text-muted-foreground">builder</span>
      <div className="flex gap-2">
        <Button size="sm" variant="outline">Export</Button>
        <Button size="sm" variant="destructive">Remove</Button>
      </div>
    </div>
  </CardContent>
</Card>
```
