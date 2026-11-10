# Design system for the Tariboy UI

Date: 2026-11-10. The current rules are in
`docs/docs/architecture/web-ui.mdx` (Design system). This document keeps the
reasons and the migration map.

## Why

The UI had tokens, shadcn/ui primitives, Storybook and a designer workflow,
but no system on top of them. An audit of `ui/src` and `ui/store` at commit
`1e492a13` found:

- Two text scales without a rule of choice, 14 sizes in use: `text-sm` (294
  uses) and `text-xs` (243) next to `text-ui` (92), `text-body-sm` (70) and
  `text-meta` (55). Steps of 0.5 px, no line height on the named sizes.
- Ten named radii from 1.5 to 14 px, plus 16 arbitrary `rounded-[Npx]`.
- Two elevation tokens written as `shadow-[var(--lift)]` (12 uses), next to 34
  uses of `shadow`, `shadow-md` and `shadow-lg`.
- No warning or success role: `amber-*` and `emerald-*` of the Tailwind palette
  in 10 files. Tints mixed by hand: six alphas of `destructive`, six of
  `muted`, 16 forms of `bg-[color-mix(...)]`.
- No layer scale: `z-50`, `z-[60]`, `z-[70]`, `z-60`.
- The dense style lived outside the kit, in `pages/tasks/panelStyles.tsx` and
  `components/metrics/tableClasses.ts`.
- The color policy covered three view files, and the pixel policy skipped
  `components/ui`.

## Goals

- One place for each decision, so a change of the look is one edit.
- A class names a token. A value in a class fails a test.
- A designer sees every token in Storybook, in both themes.

Not in scope: a split of containers into view files, a replacement of the 101
raw `<button>` elements, and stored screenshot baselines. Each is its own
task.

## Decisions

1. Three layers. `ui/src/styles/tokens.css` holds the values of both themes:
   hue and tint knobs, then roles. The `@theme` blocks of `ui/src/index.css`
   hold the scales and map a role to a utility. `ui/src/components/ui/` holds
   the primitives and `recipes.ts`.
2. The Tailwind default scales are cleared (`--text-*: initial`, and the same
   for radius, shadow and color). A class outside the system produces no CSS,
   and `tokenPolicy.test.ts` reports it.
3. Text has six steps with line heights: caption 11/16, ui 12/16, body 13/20,
   title 16/24, heading 18/28, display 24/32. The base is 13 px, as the
   console handoff (`ui/design_handoff_console_slice/`) set it. `<body>` takes
   `text-body`.
4. Radius has five steps named by use: mark 2, chip 6, control 8, card 12,
   panel 14.
5. Elevation stays at two levels and gets utilities: `shadow-raise`,
   `shadow-lift`.
6. One red. `--status-failed` is removed; a failed status is `--destructive`
   at `oklch(56% 0.17 22)`, between the two old reds.
7. `--warning` and `--success` are new roles. `--terminal` and `--scrim`
   replace `zinc-950` and `black/10`.
8. Each tone has three tints (`-soft`, `-mid`, `-line`). Their strength is a
   knob with a higher value in the dark theme, so the `dark:` overrides of a
   tint are removed.
9. Layers are `z-sticky`, `z-overlay`, `z-dialog`, `z-alert`.
10. The kit under `components/ui` is ours, not upstream-shaped: the policy
    checks it like any other code.
11. The class vocabulary of the task panel becomes `components/ui/recipes.ts`.
    It stays a set of strings: each is applied to different elements with
    local layout added by `cn()`, and about 200 call sites keep working with a
    changed import path.
12. Story ids do not change. Foundations stories have explicit titles, the
    other titles still come from the file paths, and `storySort` orders the
    sidebar.

## Migration map

| Before | After |
| --- | --- |
| `text-nano`, `text-micro`, `text-tiny`, `text-meta` (9 to 11.5 px) | `text-caption` (11) |
| `text-xs` (12) | `text-ui` (12) |
| `text-body-sm` (12.5), `text-sm`, `text-body-lg` (14) | `text-body` (13) |
| `text-title` (15), `text-base` (16) | `text-title` (16) |
| `text-lg` (18), `text-xl` (20) | `text-heading` (18) |
| `text-2xl` (24) | `text-display` (24) |
| `rounded-hair`, `rounded-xs` | `rounded-mark` |
| `rounded` (4), `rounded-chip` (5), `rounded-sm` (6) | `rounded-chip` (6) |
| `rounded-control` (7), `rounded-md` (8), `rounded-panel` (9) | `rounded-control` (8) |
| `rounded-lg` (10) on a control | `rounded-control` (8) |
| `rounded-lg` (10) on a box, `rounded-card` (12) | `rounded-card` (12) |
| `rounded-xl`, `rounded-[var(--panel-radius)]` (14) | `rounded-panel` (14) |
| `shadow`, `shadow-sm`, `shadow-[var(--raise)]` | `shadow-raise` |
| `shadow-md`, `shadow-lg`, `shadow-[var(--lift)]` | `shadow-lift` |
| `z-50`; `z-[60]`; `z-[70]`; `z-10`, `z-20` | `z-overlay`; `z-dialog`; `z-alert`; `z-sticky` |
| tone at 5 to 14 % | `-soft` |
| tone at 15 to 30 % on a fill or a ring | `-mid` |
| tone at 25 to 50 % on a border | `-line` |
| `bg-muted/10` to `bg-muted/60` | `bg-subtle` |
| `bg-primary/80` | `bg-primary-hover` |
| `status-failed` | `destructive` |
| `amber-*`; `emerald-*`, `green-500` | `warning`; `success` |
| `tracking-[.01em]`, `[.02em]`; `[.04em]`, `[.06em]`; `[-.01em]` | `tracking-wide`; `tracking-wider`; `tracking-tight` |

The visible effect: text of 14 px is 13 px with the same 20 px line, a default
button and input have a radius of 8 px, not 10, bordered boxes have 12 px, not
10, and warnings take the amber of the theme, not of the Tailwind palette.

## Verification

Screenshots of every story in both themes at 1024 px were taken before and
after the migration and compared, and the differences were read one by one.
The policy tests, the unit tests, the Storybook build and `make
frontend-check` pass.
