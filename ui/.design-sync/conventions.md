## How to build with Tariboy UI

Tariboy UI is the component library of the Tariboy desktop app (Tauri shell, web
UI). It is **shadcn/ui on Tailwind v4**, neutral grayscale, Geist Variable, built
for dense operator tooling: agent consoles, task trees, image and host tables.
Designs should read as a control surface, not a marketing page.

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
`--chart-1…5`. All 31 are redefined under `.dark`.

**Density and scale.** Body text is `text-sm`; secondary meta is `text-xs`;
headings `text-base`/`text-lg` with `font-medium`/`font-semibold`. Identifiers —
task keys, image refs, digests, paths, terminal output — are `font-mono`.
Controls are short: buttons `h-8` by default (`h-7` sm, `h-6` xs), inputs `h-8`.
Spacing is `gap-1`/`gap-2`/`gap-3` and `p-3`/`p-4`; radius comes from
`--radius: 0.625rem` via `rounded-lg`/`rounded-md`/`rounded-xl`. Icons are
**lucide-react**, 16px at default control size, 14px at sm — never emoji.

**One caveat that matters.** This bundle ships Tailwind's *compiled* output, so
only utilities the app itself already uses are present (about 790 classes).
Common layout, spacing, color, type and border utilities are all there; exotic or
rarely-used ones (e.g. `text-4xl`, `animate-pulse`, `tracking-tight`) are not.
If a utility appears to do nothing, express it as an inline style with
`var(--token)` instead.

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
