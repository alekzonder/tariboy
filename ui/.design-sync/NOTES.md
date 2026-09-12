# design-sync notes — tariboy-ui

## Repo shape

- `ui/` is an **application**, not a published component library: `package.json` is
  private with no `main`/`module`/`exports`, there is no `dist/` and no `.d.ts`
  tree. Consequences, all already wired in `config.json`:
  - The bundle entry is **generated**: `.design-sync/make-entry.mjs` writes
    `.design-sync/entry.tsx`, which re-exports every module under
    `src/components/ui/` and `src/components/` (plus `MemoryRouter` from
    react-router-dom, for previews of router-coupled components). Re-run it
    after adding or deleting a component file. Pass it via
    `--entry ./.design-sync/entry.tsx`.
  - Because there is no `.d.ts` tree, the converter discovers **nothing** on its
    own — every card-level component must be listed in `cfg.componentSrcMap`.
    Adding a component to the repo does NOT add it to the sync automatically.
  - Do **not** let the converter fall back to synth-entry (drop `--entry`): it
    would `export *` every `.tsx` under `src/`, dragging pages, routing and
    `App.tsx` into the bundle.

## CSS and fonts

- Tailwind v4 is compiled by the Vite plugin at app-build time; the repo ships no
  static stylesheet. `.design-sync/prepare-css.sh` runs the real desktop build and
  stages `desktop/dist/assets/*.css` + the Geist `.woff2` files into
  `.design-sync/.cache/css/` (flat, because the `@font-face` urls are same-dir
  relative). `cfg.cssEntry` points there. `cfg.buildCmd` runs this plus
  `make-entry.mjs`, so a re-sync only needs `buildCmd`.
- **Known limitation:** that stylesheet contains only the utility classes the app
  itself uses. Tailwind generates on demand, so a utility the design agent invents
  but the app never used will not resolve. Tokens (`:root` / `.dark`) ship whole,
  so `var(--token)` always works. Revisit if designs come back unstyled in places.

## Preview authoring (calibrated on Button, Card, Select)

- `import { X } from "tariboy-ui"` in a preview resolves to the shipped bundle
  global — that is the correct import spelling.
- `lucide-react` imports work in previews (bundled from node_modules). The DS uses
  lucide everywhere; never draw substitute glyphs.
- Radix `open`/`defaultOpen` renders the portal content **inside** the card —
  `Select` with `open` needed no `cardMode` override. Try that before reaching for
  `cfg.overrides`.
- CSS custom properties (`var(--border)`, `var(--muted-foreground)`) are available
  to inline styles in previews.
- Realistic content comes from the product: agents (`builder`, `reviewer`,
  `packager`, `docs-bot`), images (`worker:v1`, `bare:latest`), hosts
  (`This daemon (local)`, `build-01`), task keys (`TB-142`).

## Component-specific

- Both resolved in the first wave; see "Preview authoring" below.

## Grouping

- All components currently land in group `general`: the converter derives the
  group from the last non-generic src path segment, and both `components` and
  `ui` are on its generic list. Fixing this properly means per-component doc files
  with `category:` frontmatter (which also replace the synthesized `.prompt.md`).

## Preview authoring — carried forward from the first wave

These are settled findings; a later wave should not rediscover them.

- **Scaffolding in previews uses inline `style` with `var(--token)`, never invented
  Tailwind utilities.** The staged stylesheet only contains classes the app itself
  emitted, so a utility written for the first time in a preview does not resolve.
  Classes that already appear in `src/components/ui/*.tsx` or app code are safe
  (Tailwind scanned those files) — e.g. `sm:max-w-2xl`, `text-destructive`.
- **Radix portals escape the card** into `document.body`, so overlay content is
  positioned against the 900x700 capture viewport rather than the card box. That
  renders correctly and needs **no** `cfg.overrides.<Name>.cardMode`. It does need
  real in-flow content behind the overlay, or the dimmed backdrop covers blank
  white and the cell reads as broken.
- `defaultOpen` works on Dialog, AlertDialog, Sheet, DropdownMenu, Collapsible.
  **`ContextMenu.Root` has no `open`/`defaultOpen`** — Radix keeps that state
  internal and driven by the pointer. `ContextMenu.tsx` opens it by dispatching a
  real `contextmenu` MouseEvent from a mount effect through an `asChild` ref;
  nothing is stubbed, and the clientX/clientY choose where the menu lands.
- `Tooltip` needs `TooltipProvider` composed inside the preview (not a global
  `cfg.provider` — only Tooltip needs it).
- Small components need deliberate sizing or they capture as invisible slivers:
  Input/Textarea want a labelled field 320-460px wide; Badge wants a flex row;
  **vertical `Separator` needs an explicit height** (`data-vertical:self-stretch`
  is inert in the `align-items:center` row it is always used in); `ScrollArea`
  needs a fixed height plus content that overflows.
- `Switch` uses `defaultChecked`; `checked` without `onCheckedChange` makes Radix
  warn and previews carry no state.
- `CommandList` is `max-h-64` and clips a ~6-item two-group palette — keep authored
  palettes to about 5 items. `CommandEmpty` does render statically with
  `shouldFilter={false}`.
- React hooks work in previews (`import { useEffect, useRef } from "react"` is
  shimmed to the bundle's React).

## Imperative module singletons must be exported from the entry

`Toaster` initially rendered an empty container: the shipped component subscribes
to sonner's `ToastState` inside `_ds_bundle.js`, while a preview importing
`"sonner"` got a **second** copy bundled into `_preview/Toaster.js`. Two
singletons, so queued toasts never reached the mounted Toaster.

Fix (in `make-entry.mjs`): the entry re-exports `toast` from sonner, putting the
bundle's own singleton on `window.TariboyUI`. Previews import it from
`"tariboy-ui"`. **Any future component whose API is an imperative module singleton
(a store, an event bus, a second toast library) needs the same treatment** — add
its imperative export to `make-entry.mjs`. Lowercase exports are never mistaken
for components.

## Components with no usage anywhere in src/

`Separator`, `SheetFooter`, `AlertDialogMedia`, `DropdownMenuLabel`,
`DropdownMenuSeparator` and `AlertDialogContent size="sm"` are exported but never
used by the app. Their previews are plausible compositions rather than lifted
usage — re-ground them if real usage lands later.

## Preview authoring — second wave (application components)

- **Most "self-fetching" components turned out to have injection seams.** Check
  the props type before assuming a component cannot be previewed:
  `ShellScriptEditor` takes `load`/`save`, `FileBrowser` takes a whole
  `FileBrowserApi`, `TuiScreen` takes the `controller` (the websocket lives
  outside it, so a captured session renders a real xterm), `IterationJudgePanel`
  renders from a `judge` projection, `PathAutocomplete` is fully controlled.
- **The failure shell is a legitimate preview.** The capture server 404s every
  `/api/...` read, so fetch-coupled components land in their genuine empty/error
  states. That is honest content, not a placeholder — grade it on the rubric.
- **Driving a component through its own DOM** is the house technique for states
  with no prop: dispatch the real event from a mount effect. `.click()` opens
  Radix Dialog/AlertDialog/Sheet and Collapsible; **Radix Select needs a
  `pointerdown` PointerEvent**, not a click; React inputs (and cmdk's
  `CommandInput`) are driven by the native value setter plus a bubbling `input`
  event. Nothing is stubbed at the network layer. Typing into a field your own
  click just revealed needs two timer ticks (~120ms click, ~260ms type).
- **A component that tints itself from data needs that data seeded, or the card
  teaches the wrong palette.** `AgentLayout`'s tab strip is `.agent-header`,
  which colours itself from `--agent-hue` (fed from the agent's stored colour).
  With no daemon behind the capture server the colour never arrives, the hue
  falls back to 0, and the strip renders PINK — a colour that appears nowhere
  in this DS and reads as an error state. `useCachedColor` falls back to
  `localStorage`, so the preview seeds `agent:color:builder` at module scope
  (the same per-cell technique `HostSwitcher` uses). Check any other component
  that derives colour from fetched data before trusting its card.

- **Each cell is captured on its own page load** (`?story=<Cell>`), so per-cell
  `localStorage` seeding is safe — that is how `HostSwitcher` gets a populated
  daemon registry (seed `tariboy_daemons` / `tariboy_active_daemon` while
  rendering a wrapper ABOVE `DaemonProvider`).
- `make-entry.mjs` does `export *` over every mapped module, so non-component
  exports (providers, helpers) are already importable from `"tariboy-ui"` —
  check that before concluding something must be added to the entry.
- `Button variant="destructive"` is deliberately soft in this DS
  (`bg-destructive/10 text-destructive`). Pale Kill/Remove buttons are correct.

- **A page a preview routes to must be in `make-entry.mjs`'s `pages` list, not
  just imported by the preview.** `SettingsPage`'s Hosts cell rendered
  `undefined` ("Element type is invalid") for a whole sync because
  `DaemonsPage` - the `/hosts` section body - was never in the entry, so
  `import { DaemonsPage } from "tariboy-ui"` resolved to nothing. The shell
  drew fine and the cell was merely blank, so it read as a data-less empty
  state rather than a break. It cost +3KB of bundle to fix. **This class of
  bug is invisible under `--no-render-check`** - it surfaces only as
  `[RENDER_ERRORS]`, which is non-blocking and needs the render check to fire
  at all. When adding a page preview, check every component it routes to
  against `_ds_bundle.js` before trusting the card.

## Components that cannot render a meaningful static preview

Documented limits, not defects — they ship importable with types and docs:

- **`UpdateBanner`** returns `null` unless a Tauri update snapshot has arrived at
  phase `ready`. `DesktopUpdatesContext` is module-private in
  `src/components/DesktopUpdates.tsx`, and stubbing `window.__TAURI_INTERNALS__`
  does not help (the dynamic `@tauri-apps/api` import rejects). To preview it,
  the app would have to export that context or accept an optional snapshot prop.
- **`CustomerQuestionNotifications`** is headless by design — its only DOM is
  `{children}`.
- **`IterationAuditLog`**'s event stream comes from `agentGet(..."logs")` +
  `fetchTranscript` with no prop seam; its cells show the real frame in the
  genuine "No events." state. An optional `events` prop would make it previewable.
- A *populated* `StatusChatHistory` needs `/api/agents/<name>/status/history`;
  the sheet renders its real "No status events." empty state instead.

## Product findings (logged here, NOT changed)

- `FileBrowser` renders markdown into `prose prose-sm dark:prose-invert`, but
  `@tailwindcss/typography` is not installed (`src/index.css` imports only
  tailwindcss, tw-animate-css, shadcn and Geist) and `prose*` appears zero times
  in the stylesheet the app ships. Rendered markdown therefore gets preflight's
  reset and no type scale in the real application. Worth a ticket.
- `src/index.css`'s `.agent-header` comment says the `--agent-hue` fallback
  "keeps the tint subtle/neutral", but hue 0 at chroma 0.035 is a visibly pink
  strip, not a neutral one. Any agent whose colour has not loaded (or is unset)
  gets a pink header in the real app. Either the fallback should be chroma 0,
  or the comment should say what it actually does. Worth a ticket.
- Components exported but never used anywhere in `src/`: `Separator`,
  `SheetFooter`, `AlertDialogMedia`, `DropdownMenuLabel`, `DropdownMenuSeparator`,
  `AttachButton`, and `AlertDialogContent size="sm"`. Their previews are
  plausible compositions rather than lifted usage.

## Known render warns (triaged — not new findings)
- Focus rings appear in overlay captures because Dialog/AlertDialog/Sheet autofocus
  their Cancel or close button. Genuine component behaviour, not a defect.
- `Badge` `ghost` and `link` variants are text-only at rest; that is correct, not
  an unstyled cell.
- Radix scrollbars are hover-type, so `ScrollArea` captures show no thumb;
  overflow reads from the clipped last row.


- **This build box has no emoji font**, so emoji written into component source
  (`ProxyCallRow`'s scroll/thinking markers, audit descriptors) capture as empty
  tofu boxes. Capture artifact only; they render normally on a real machine.

## Token classification: `/* @kind other */` (fork of lib/css.mjs)

The design-system token scan reads every custom property reachable from
`styles.css`. Tariboy ships no static stylesheet, so `_ds_bundle.css` is the
only place its CSS lives, and the scan saw ~325 declarations that are not
design tokens sitting next to the 87 real ones:

- **73 `--tw-*`** Tailwind v4 internals - on `*,:before,:after,::backdrop`, on
  utility classes, and as 73 `@property` rules.
- **57 flexlayout-react vars** from its `light.css`, declared on
  `.flexlayout__layout` under *generic* names that collide with real token
  names: `--color-text`, `--color-background`, `--font-family`, `--size-*`.
  Note there is no `--flexlayout*` prefix - filtering by name would have
  missed all of them, which is why the rule is structural.
- **3 `--splitter-*`** from `terminalWorkspace.css`, plus `--card-spacing`
  from a Tailwind arbitrary-property utility.

`.design-sync/annotate-tokens.mjs` marks them with `/* @kind other */`. The
rule is **scope-based, not a name allowlist**: a custom property whose
innermost selector is not a theme scope (`:root`, `:root,:host`, `.dark`,
`html`, `:host`) is not a design token. The 87 tokens under `:root`/`.dark`
(`--background`, `--primary`, `--sidebar*`, `--chart-1…5`, …) are untouched
and still classify normally.

Nothing is deleted - `--tw-*` in particular is load-bearing for the compiled
utilities. The pass is metadata only, and is verified so: the annotated CSS is
byte-identical to the original once comments are stripped. It is also
idempotent (it strips every marker it has written, then re-derives them).

**Why it is a fork and not a `buildCmd` step.** `cfg.buildCmd` runs *before*
the converter, and annotating `cfg.cssEntry` alone covers only 248 of the 325
declarations: the converter appends ~48KB of CSS that esbuild pulls out of the
JS bundle (flexlayout, xterm), which never passes through `cssEntry`. So the
annotation runs from `.design-sync/overrides/css.mjs` at the top of
`writeStylesCss`, which `package-build.mjs` calls after `_ds_bundle.css` is
final and **before** `styleShaFor()` - so the anchor describes the CSS that
actually ships. `cfg.libOverrides` declares the fork.

Two things to know about the fork:

- It needs `.design-sync/node_modules` (the `ln -sfn ../.ds-sync/node_modules`
  symlink) on a fresh clone, because it imports from `../../.ds-sync/lib/`.
- **Introducing or editing it re-opens verification for every component.**
  `configSlicesFor` in `lib/sync-hashes.mjs` hashes the *bytes* of every
  `.design-sync/overrides/*.mjs` into the global slice of each component's
  `sourceKey` (`cfg.libOverrides` prose is deliberately NOT keyed - the file
  bytes are). So the sync that added this fork saw all 67 components flip from
  `unchanged` to `changed` and every local grade clear. That is by design, not
  nondeterminism: a lib fork can change any render. A later sync that leaves
  the fork alone carries everything forward again - if grades clear when the
  fork has NOT changed, that IS a real finding, chase it.

If the check still reports unclassified tokens after this, the remaining names
will be real `:root`/`.dark` tokens whose *values* it cannot bucket (easings,
durations, font weights, `calc()` line-heights) - a different problem from the
scope pollution this pass fixes, and one to solve by value, not by scope.

## Page-level surfaces, and why the render check is skipped

`src/pages` surfaces ship as cards too: `TerminalsPage` (the app shell),
`AgentWorkspace`, `TasksWorkspace`, `TaskDetail`, `SettingsPage`, `ImagesPage`,
`StoresPage`. They give the design agent whole-page compositions — real density
and layout — rather than only atoms. `make-entry.mjs` re-exports exactly those
seven, deliberately NOT a walk of `src/pages`: every module in the entry lands in
the shipped bundle.

Recipe (calibrated on `TerminalsPage.tsx` — read it before writing another page
preview):

- Provider stack mirroring `src/App.tsx`, minus anything that opens a socket:
  `DaemonProvider` > `SidebarStateProvider` > `DesktopUpdatesProvider`.
  **`CustomerQuestionNotifications` must stay out** — it opens a tasks websocket
  that never closes, and the render check's `networkidle` wait then never
  settles (the card times out). Its context has a usable default.
- A **matched route**: `MemoryRouter` alone matches nothing, and these screens
  derive their identity from `useParams()`. `SettingsPage` also needs child
  routes for its `<Outlet/>`.
- A fixed-size frame (620x1180) — screens collapse without a height — and
  `cardMode: "column"` (or `single` for `TaskDetail`, whose drawer portals to
  body and would stack all stories at one fixed position).
- Clear `tariboy_daemons` / `tariboy_active_daemon` at module scope. The render
  check reuses ONE browser page for every card, so `HostSwitcher`'s preview
  leaves a seeded registry behind; a page that inherits it resolves three
  unreachable SSH hosts and polls all of them.

**`--no-render-check` is now part of the final validate, by the repo owner's
decision.** Cause, measured rather than assumed: the pages push `_ds_bundle.js`
from 3.5 MB to 5.4 MB, and at that size Playwright's `networkidle` fires before
V8 finishes parsing the bundle and React commits. The FIRST card checked in a run
therefore reports `root empty` — it is always the first one, whichever that is
(`AgentWorkspace` while it led the list, then `AgentColorSwatch`, which makes no
network request at all). Direct measurement: root length 0 at networkidle, 80515
two seconds later, no page errors. 65 of 66 cards pass in every run; the pages
were additionally verified by hand at full width (see the shot helper below).
Trimming the entry to seven pages only moved 5603 -> 5437 KB, so the weight is
the pages' own dependency graph (flexlayout, the dialogs, the task tree), not the
breadth of the re-export.

If a future sync wants the machine check back, the lever is bundle size: drop the
page surfaces from `make-entry.mjs` and the check goes clean again at ~3.5 MB.

`.design-sync/.cache/shot.mjs <html> <out.png> <w> <h>` screenshots one card at
full width — the review sheets crop at ~900px, which is narrower than a page
frame, so use it whenever a page card needs judging.

## Re-sync risks

- `.design-sync/overrides/css.mjs` is a **fork of the skill's `lib/css.mjs`**.
  On re-sync, diff it against the bundled `lib/css.mjs` and merge upstream
  changes - the fork is a verbatim copy plus two edits (the import repoint and
  one `annotateFile()` call at the top of `writeStylesCss`). Editing it clears
  every grade (see the token-classification section); leaving it alone does
  not.
- The fork imports from `../../.ds-sync/lib/`, so a fresh clone needs
  `ln -sfn ../.ds-sync/node_modules .design-sync/node_modules` before the
  converter runs.
- `annotate-tokens.mjs`'s theme-scope list (`:root`, `:root,:host`, `.dark`,
  `html`, `:host`) is what separates tokens from noise. If the app ever moves
  its tokens to another scope (a `[data-theme]` attribute, a `@layer theme`
  block, a media-query dark mode), that scope must be added or all 87 real
  tokens get demoted to `@kind other` in one silent step. The build log line
  is the tripwire: it prints how many were kept as tokens - if that number
  falls off 87 without a deliberate token change, stop.

- The final validate runs with `--no-render-check` (see above). If the bundle
  ever shrinks back under roughly 4 MB, drop the flag and confirm a clean run.
- `AgentWorkspace` has a card again; if it or any other page starts failing the
  gate differently, re-read the section above before assuming a regression.
- **A driver run can fail spuriously if anything else is running alongside it.**
  One closing run reported five previews as `Could not resolve "lucide-react"` /
  `"sonner"` (one even failed on a RELATIVE import inside lucide) and then
  `playwright not installed` — while every one of those packages was present and
  resolvable on disk, and an immediate `preview-rebuild` of the same five
  succeeded with no changes. Treat "Could not resolve" for a package you can
  `ls` as a transient, and re-run before believing it. Do NOT ship the build:
  a failed preview silently drops that component to the floor card and the run
  still exits with the anchor written.
- **Never wait on a run with `while pgrep -f resync.mjs`** — `pgrep -f` matches
  its own command line, so the loop never exits, spins until the tool timeout,
  and leaves a stray shell spawning `pgrep`/`sleep` for the rest of the session.
  Two of those were live during the spurious failure above. Use the shell tool's
  background mode and wait for its completion notification instead.
- `componentSrcMap` is a hand-maintained enumeration of 60 card components. It
  silently goes stale when components are added or renamed — diff it against
  `src/components/ui/*.tsx` and `src/components/*.tsx` on every sync.
- `make-entry.mjs` must keep emitting: named re-exports for **default**-exported
  components (`export *` does not forward a default — validate catches this as
  `[BUNDLE_EXPORT]`), the imperative `toast` singleton, the agent contexts, and
  `MemoryRouter`/`Routes`/`Route`.
- Four components are knowingly below a rich preview (see the section above).
  Re-check them if the app ever exposes the seams named there.

- `componentSrcMap` is a hand-maintained enumeration — it silently goes stale when
  components are added or renamed in the repo. Diff it against
  `src/components/ui/*.tsx` and `src/components/*.tsx` on every sync.
- `prepare-css.sh` depends on the desktop Vite build succeeding and on
  `desktop/dist/assets/` holding exactly one `.css`. A build change breaks CSS
  silently (validate would report `[CSS_PLACEHOLDER]` or missing tokens).
- The staged CSS is gitignored (`.design-sync/.cache/`), so a fresh clone must run
  `buildCmd` before the converter.
