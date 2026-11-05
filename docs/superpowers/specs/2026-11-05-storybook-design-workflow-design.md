# Storybook as the designer workplace and the designer-developer contract

Date: 2026-11-05. Stage 1 of 7. This document is the specification. The
implementation plan for stages 2 to 7 is
`docs/superpowers/plans/2026-11-05-storybook-design-workflow.md`.

The goal: a designer agent works on the UI visually in Storybook, with
component variants and whole pages on fixtures. A developer agent connects
the same components to data and logic. Stories and page mockups are in the
repository and compile against the real code, so a change of props breaks the
build. The brief has seven accepted decisions (decision 1 to decision 7). This
document keeps them. Section 12 lists each difference and its reason.

All facts below come from the repository at commit `0bef8722` and from
commands run on 2026-11-05. The commands are in the sections that use them.

## 1. Layers and ownership

The table of decision 2, with the exact paths:

| Layer | Paths | Designer | Developer |
| --- | --- | --- | --- |
| Tokens and primitives | `ui/src/index.css`, `ui/src/components/ui/*.tsx` | Owns colors, radii, density, shadows and the `cva()` variants | Changes only accessibility and behavior (focus, keyboard), after agreement with the designer |
| Product view components | `ui/src/**/*.view.tsx` (section 2) | Owns markup and styles. Proposes a new prop in `handoff.md` | Owns the props type as the contract. Connects the view to data in a container |
| Containers | every other `ui/src/components/**/*.tsx` and `ui/src/pages/**/*.tsx` | Does not change them. For a component without a view file, copies its markup into a local view part of a mockup (see below) | Owns data hooks, `@/lib/api`, react-query, router hooks, state and handlers |
| Pages | mockup: `ui/src/mockups/<page>/`; real page: `ui/src/pages/**` | Builds the mockup page from view components on fixtures | Owns routing, requests, state and handlers of the real page |
| Component mockups | `ui/src/mockups/<component>/` (section 5) | Builds the story `Mockups/<Component>` and its local view part | Moves the change into the real component |
| Stories | `ui/src/**/*.stories.tsx`, next to the component | Writes and changes them | Changes a story only when a props change breaks it, and mentions `@agent:tariboy-designer` on the task of the change |
| Fixtures | `ui/src/fixtures/<domain>.ts` (section 5) | Writes and changes them | Keeps them typed against `@/types` when a type changes |
| Handoff | `ui/src/mockups/<page>/handoff.md` or `ui/src/mockups/<component>/handoff.md` (sections 5, 6) | Writes it | Reads it, answers the open questions, and implements it |
| Types | `ui/src/types/` (section 4) | Reads them | Owns them |
| Storybook setup | `ui/.storybook/`, `ui/src/storybook/` | Proposes a new decorator | Owns the configuration and the decorators |

A designer change touches only the designer columns. A developer change that
breaks a story fixes the story in the same commit.

A component without a view file. After this project, `AgentRow.view.tsx` is
the only view file (section 2), and mass separation is out of scope. So most
components are containers, and the designer does not change them. To change
such a component, the designer copies its markup into a local view part
`ui/src/mockups/<page>/<Name>.view.tsx` (or
`ui/src/mockups/<component>/<Name>.view.tsx`), changes it there, shows it in
the mockup story, and asks in `handoff.md` to split the component. The
developer splits the component (section 7.2) and moves the change into the
new view file. This follows from decision 2 and the ban on mass separation.
It is not a difference from them.

## 2. Mark of a view file

Choice: the file name suffix `.view.tsx`. Example:
`ui/src/components/AgentRow.view.tsx`. The story of that file is
`AgentRow.stories.tsx` in the same directory.

Reasons:

- A component moves to the view layer with one rename:
  `git mv AgentRow.tsx AgentRow.view.tsx`, plus the import paths that name the
  file. The file stays in its domain directory (`audit/`, `role/`,
  `file-browser/` and the others). Mass separation is out of scope, so the
  mark must let us move components one at a time. A rename does that.
- The glob `src/**/*.view.tsx` is a direct value for oxlint `overrides`
  (section 3 tested it with the same glob form) and for the policy test.
- The importers do not see a difference in the module name for the default
  case: `import { AgentRow } from "@/components/AgentRow.view"`. The suffix
  in the import shows the reader that the module is a view.

Rejected options:

- A `views/` directory. It separates a component from its test, its story
  and its container, and a move to the view layer is a move of a file into
  another directory with a new import path for every importer.
- A list of files in `.oxlintrc.json`. It is neither a directory nor a name
  rule, nothing marks the file itself, and a new view file is easy to leave
  out of the list.

`AgentRow.tsx` is already a pure view (it imports only `react` types,
`@/components/ui/status`, `@/lib/agentState`, `@/lib/utils` and
`@/pages/home/homeModel`). Stage 5 renames it to `AgentRow.view.tsx` as the
first view file, so the lint override has a real file to apply to.

## 3. Lint rules of decision 5

Decision 5: in the view layer, stories and mockups, these are forbidden:

- every import from `@/lib/api`, `import type` included;
- `@tanstack/react-query`;
- `fetch`;
- hex colors and Tailwind palettes in classes.

`@/types` is allowed.

### 3.1 Test runs on oxlint 1.81.0

All runs used `ui/node_modules/.bin/oxlint` from the main checkout
(`oxlint --version` prints `Version: 1.81.0`). The temporary files were in
`ui/.lint-probe/` of the task worktree. The directory was deleted after the
runs and was not committed. Each run used the host check slot:
`flock -o /tmp/tariboy-check.lock`.

The final test configuration `ui/.lint-probe/b.json` (command 2 describes
the first variant, `ui/.lint-probe/.oxlintrc.json`):

```json
{
  "plugins": ["typescript", "react"],
  "categories": { "correctness": "error" },
  "overrides": [
    {
      "files": [
        "src/**/*.view.tsx",
        "src/**/*.stories.tsx",
        "src/mockups/**",
        "src/fixtures/**"
      ],
      "rules": {
        "no-restricted-imports": ["error", {
          "paths": [
            { "name": "@tanstack/react-query", "message": "view layer: no data fetching" }
          ],
          "patterns": [
            {
              "group": ["**/lib/api", "**/lib/api/*", "@tanstack/react-query/*"],
              "message": "view layer: props in, JSX out; types from @/types"
            }
          ]
        }],
        "no-restricted-globals": ["error", { "name": "fetch", "message": "view layer: no network calls" }],
        "no-restricted-properties": ["error",
          { "object": "window", "property": "fetch" },
          { "object": "globalThis", "property": "fetch" },
          { "object": "self", "property": "fetch" }
        ]
      }
    }
  ]
}
```

The test files:

- `src/components/agent/Probe.view.tsx` with these lines: 1
  `import { agentsApi } from "@/lib/api"`, 2 `import type { Agent } from
  "@/lib/api"`, 3 `import { tasksApi } from "@/lib/api/tasks"`, 4
  `import { useQuery } from "@tanstack/react-query"`, 5 `import type {
  QueryClient } from "@tanstack/react-query"`, 6 `import { x } from
  "../../lib/api"`, 7 `import type { Ok } from "@/types"`, 8 `export type {
  Task } from "@/lib/api"`, 9 `export { agentsApi as again } from
  "@/lib/api"`, 10 `export * from "@tanstack/react-query"`, and in the body
  `fetch("/a")` (line 17), `window.fetch("/b")`, `globalThis.fetch("/c")`,
  `self.fetch("/d")` and
  `className="bg-red-500 text-[#fff] border-primary"`;
- `src/components/agent/Other.tsx`, not a view file, with the same imports,
  `fetch`, `window.fetch` and `className="bg-red-500 text-[#fff]"`;
- `src/mockups/home/Home.stories.tsx` with `import { tasksApi } from
  "@/lib/api/tasks"`, `cn("p-2", "border-[#ff0000]", …)`, a `cva()` variant
  `"bg-rose-600 text-white"` and `style={{ color: "#123456" }}`;
- `src/fixtures/agents.ts` with `import type { Agent } from "@/lib/api"` and
  `import type { Agent as Good } from "@/types"`.

Command 1, from `ui/.lint-probe`:

```bash
flock -o /tmp/tariboy-check.lock /home/agent/github/tariboy/ui/node_modules/.bin/oxlint -c b.json -f unix .
```

Result: exit code 1, 11 `no-restricted-imports` errors, one for each
forbidden import line (`grep restricted-imports | cut -d' ' -f1,2`, so each
line shows the file, the position and the module):

```text
src/fixtures/agents.ts:1:1: '@/lib/api'
src/components/agent/Probe.view.tsx:1:1: '@/lib/api'
src/components/agent/Probe.view.tsx:2:1: '@/lib/api'
src/components/agent/Probe.view.tsx:3:1: '@/lib/api/tasks'
src/components/agent/Probe.view.tsx:4:1: '@tanstack/react-query'
src/components/agent/Probe.view.tsx:5:1: '@tanstack/react-query'
src/components/agent/Probe.view.tsx:6:1: '../../lib/api'
src/components/agent/Probe.view.tsx:8:1: '@/lib/api'
src/components/agent/Probe.view.tsx:9:1: '@/lib/api'
src/components/agent/Probe.view.tsx:10:1: '@tanstack/react-query'
src/mockups/home/Home.stories.tsx:3:1: '@/lib/api/tasks'
```

and four errors for `fetch`:

```text
src/components/agent/Probe.view.tsx:17:8: Unexpected use of 'fetch'. view layer: no network calls [Error/eslint(no-restricted-globals)]
src/components/agent/Probe.view.tsx:18:8: 'window.fetch' is restricted from being used. [Error/eslint(no-restricted-properties)]
src/components/agent/Probe.view.tsx:19:8: 'globalThis.fetch' is restricted from being used. [Error/eslint(no-restricted-properties)]
src/components/agent/Probe.view.tsx:20:8: 'self.fetch' is restricted from being used. [Error/eslint(no-restricted-properties)]
```

What this result shows:

- `import`, `import type`, `export type { … } from`, `export { … } from` and
  `export * from` are all caught (lines 1, 2, 8, 9, 10).
- The pattern `**/lib/api` catches the alias `@/lib/api` and the relative path
  `../../lib/api`. The pattern `**/lib/api/*` catches the domain module
  `@/lib/api/tasks`.
- Line 7, `import type { Ok } from "@/types"`, gives no error.
- `Other.tsx` gives no error. The override applies only to the files of its
  glob.
- No error names a hex color or a Tailwind palette class.
- In the default output format, the pattern message is printed as
  `help: view layer: props in, JSX out; types from @/types`.

Command 2, the first variant of the configuration, run as
`oxlint -f unix .` with `ui/.lint-probe/.oxlintrc.json`: `paths` had
`@/lib/api` and `@tanstack/react-query`, and `patterns` had `@/lib/api/*`,
`@tanstack/react-query/*`, `**/lib/api` and `**/lib/api/*`. The other rules
were the same. Result: exit code 1, `20 problems`: 16 import errors and the
same 4 `fetch` errors. Each `@/lib/api` line got two errors, "is restricted
from being used" (the path) and "is restricted from being used by a pattern".
The final configuration removes `@/lib/api` from `paths`, and each forbidden
line then gets one error.

Command 3 is the reading of the `fetch` lines of commands 1 and 2.
`no-restricted-globals` reports only line 17, `fetch("/a")`. Lines 18–20
(`window.fetch`, `globalThis.fetch`, `self.fetch`) get an error only from
`no-restricted-properties`. Without that rule, the three member calls pass.
For this reason the configuration has both rules.

Command 4, the rule list:

```bash
oxlint --rules -f json | jq -r '.[] | "\(.scope)/\(.value)"' | grep -i -E 'tailwind|color|hex|restricted|syntax'
```

Result: 870 rules. The matches are only `no-restricted-exports`,
`no-restricted-globals`, `no-restricted-imports`, `no-restricted-properties`,
`no-shadow-restricted-names`, `import/no-webpack-loader-syntax`,
`jest/no-restricted-jest-methods`, `jest/no-restricted-matchers`,
`react/syntax`, `react/unsupported-syntax`, `typescript/no-restricted-types`,
`unicorn/no-hex-escape`, `vitest/no-restricted-matchers` and
`vitest/no-restricted-vi-methods`. No rule checks Tailwind classes or hex
colors in strings. (`unicorn/no-hex-escape` is about `\x` escapes.)

Command 5, `no-restricted-syntax`:

```bash
echo '{"rules":{"no-restricted-syntax":["error","Literal"]}}' > syn.json
oxlint -c syn.json src/fixtures
```

Result: `Failed to parse oxlint configuration file.` and
`x Rule 'no-restricted-syntax' not found in plugin 'eslint'`. A selector rule
for string literals is not available.

Command 6: `grep -o '"jsPlugins"' ui/node_modules/oxlint/configuration_schema.json`
finds the key. oxlint can load JavaScript plugins, but none is installed.

### 3.2 Implementation

| Rule | Implementation | Reason from 3.1 |
| --- | --- | --- |
| No import from `@/lib/api`, `import type` and `export … from` included | oxlint `no-restricted-imports`, `patterns` `**/lib/api`, `**/lib/api/*` | Command 1 |
| No `@tanstack/react-query` | oxlint `no-restricted-imports`, `paths` `@tanstack/react-query`, pattern `@tanstack/react-query/*` | Command 1 |
| No `fetch` | oxlint `no-restricted-globals` for `fetch`, and `no-restricted-properties` for `window.fetch`, `globalThis.fetch`, `self.fetch` | Commands 1 and 3 |
| No hex colors and no Tailwind palettes | Policy test `ui/src/test/viewLintPolicy.test.ts` | Commands 1, 4, 5 |
| Scope | One oxlint `overrides` entry with `src/**/*.view.tsx`, `src/**/*.stories.tsx`, `src/mockups/**`, `src/fixtures/**` | Command 1 (`Other.tsx`) |

The override goes into `ui/.oxlintrc.json`. The `rules` of that entry are
exactly the three rules of 3.1. `ui/src/test/lintPolicy.test.ts` keeps the
expected list of overrides with rules, so it gets this entry as a third
expected value (stage 5).

The policy test `ui/src/test/viewLintPolicy.test.ts`:

- It collects the files of the same four globs under `ui/src` (excluding
  `node_modules`). The glob list is read from the override in
  `.oxlintrc.json`, so the two scopes cannot differ. It reads only `.ts` and
  `.tsx` files: a `handoff.md` in `src/mockups/` can name a hex value of a
  new token pair in its text.
- It reads the palette names from
  `ui/node_modules/tailwindcss/theme.css`: every `--color-<name>-<n>`. With
  Tailwind 4.3.0 these are 26 names: `amber blue cyan emerald fuchsia gray
  green indigo lime mauve mist neutral olive orange pink purple red rose sky
  slate stone taupe teal violet yellow zinc`. A Tailwind update that adds a
  palette adds it to the test.
- It searches the whole text of each file for:
  - a hex color `#[0-9a-fA-F]{3,8}\b`;
  - an arbitrary color value `-[#`;
  - a palette class `\b[a-z-]*-(<palette names>)-\d{2,3}\b` (any prefix:
    `bg-`, `text-`, `border-`, `ring-`, `from-`, `fill-` and the others, with
    variants such as `hover:` before it and an opacity such as `/50` after it);
  - a palette variable `--color-(<palette names>)-\d{2,3}`.
- It fails with a list of `file:line: match`.
- It has a self-test: it writes a temporary directory with one file for each
  case of 3.1 (`className`, `cn()`, `cva()`, `style`) and expects four
  findings, and one clean file with `bg-primary text-muted-foreground` and
  expects none.

A search of the whole text is stricter than a parse of `className`, `cn()`
and `cva()`. It does not depend on the form of the call, and it also covers
inline `style` values. A hex value in a comment of a view file also fails.
That is deliberate. A search of the current 55 previews finds only
`SendFilesButton.tsx` lines 17 and 18 (`var(--color-zinc-950, #09090b)` and
`#d4d4d8`). Section 9 says how the migration removes them.

`jsPlugins` is not used. A plugin is a new dependency surface for one rule,
and the policy test covers the rule without it.

### 3.3 What the rules do not cover

- The rules check the direct imports of a file. A story of a container
  imports the container, and the container imports `@/lib/api`. That is
  allowed (decision 5 limits the file, not its dependency graph).
- Router hooks (`useNavigate`, `useParams`, `useLocation`,
  `useSearchParams`) in a view file are forbidden by decision 3, but no lint
  rule checks them. Review checks them (section 7). A view can render
  `<Link to={href}>`, because the Storybook preview gives every story a
  `MemoryRouter`.
- Tailwind `black` and `white` (`text-white`) are not palettes with a scale.
  The test does not forbid them.
- The rules are verified for oxlint 1.81.0. After an oxlint update, stage 5's
  test shows whether they still catch each case.

## 4. Structure of `@/types`

- Directory: `ui/src/types/`.
- One file for each domain module of `ui/src/lib/api/` that re-exports types:
  `agents.ts`, `audit.ts`, `daemon.ts`, `files.ts`, `groups.ts`,
  `harnesses.ts`, `imageSources.ts`, `images.ts`, `iterations.ts`, `mail.ts`,
  `metrics.ts`, `onboarding.ts`, `operations.ts`, `plugins.ts`, `scripts.ts`,
  `stores.ts`, `tasks.ts`, `usage.ts`, `workflows.ts` (19 files). `client.ts`,
  `hosts.ts` and `index.ts` have no domain types.
- `ui/src/types/index.ts` re-exports all 19 files with `export type * from
  "./<domain>"`.
- Alias: none is added. `@/*` → `./src/*` is already in `ui/tsconfig.json`,
  `ui/tsconfig.app.json`, `ui/vite.config.ts` and
  `ui/vite.desktop.config.ts`, so `@/types` and `@/types/tasks` resolve now.
  Section 12 names this difference with decision 4.
- Content: only type re-exports from `@/lib/api-types.generated`, such as
  `export type { AgentRow, AgentView } from "@/lib/api-types.generated"`,
  and the hand-written types that `docs/docs/architecture/web-ui.mdx`
  ("API types") allows. Each hand-written type moves from its domain module
  with its comment that says why the spec does not cover it. Examples:
  `AgentEvent`, `TaskEventHint`, `MailHint`, `TaskFilters`,
  `CreateAgentSpec`, `TaskPriority`, the `audit.ts` transcript types
  `Block`, `Message`, `Call`, `RawCall`. The renamed re-exports stay renamed:
  `TaskComment` and `TaskEvent` in `types/tasks.ts`.
- What stays in `ui/src/lib/api/`: the runtime and the client types:
  `ApiError` (a class), `Transport`, `ApiClient`, `ApiTarget`, and the factory
  types such as `TasksApi = ReturnType<typeof tasksApi>`. A narrowing function
  such as `taskPriority(task)` is runtime code and stays.
- `@/lib/api` keeps all its type exports. Each domain module changes its
  `export type { … } from "../api-types.generated"` to `export type * from
  "@/types/<domain>"`, or to a named list when the module renames a type, so
  every existing import keeps working.
- Test "only types, no runtime": `ui/src/test/typesOnly.test.ts`. For each
  file in `ui/src/types/`, it runs `ts.transpileModule` (the `typescript`
  devDependency) with `verbatimModuleSyntax: true` and expects output that
  is empty after it removes `export {};` and blank lines. A `const`, a
  `function`, an `enum` or a value re-export gives JavaScript and fails the
  test. A second case expects that each domain file of `ui/src/lib/api/`
  that has a type file re-exports from `@/types/<domain>` (text search for
  `from "@/types/<domain>"`).

## 5. Fixtures and page mockups

- Shared story fixtures: `ui/src/fixtures/<domain>.ts`, for example
  `ui/src/fixtures/agents.ts`, `ui/src/fixtures/tasks.ts`. Each fixture is
  typed with `@/types` and is a complete response value (the rule of
  "API types": "Test data is a complete response"). Values come from the
  product vocabulary: agents `builder`, `reviewer`, `packager`, `docs-bot`;
  images `worker:v1`, `bare:latest`; hosts `This daemon (local)`,
  `build-01`; task keys such as `TB-142`.
- Factories. Each domain fixture file exports a factory
  `<entity>(over: Partial<T> = {}): T` with complete defaults, for each
  entity that stories use. A designer writes only the fields that matter:
  `agentRow({ name: "builder", state: "running" })`. `AgentRow`, `Task` and
  `MetricsReport` have tens of fields, and a full literal in each story is
  hard to write and to keep up to date. The form is the form of the
  factories in `ui/src/test/fixtures/apiFixtures.ts` today: `agentRow`,
  `agentView`, `imageSummary`, `operation`, `taskItem`, `metricsSlice`,
  `metricsReport`, `taskMetrics` and the others.
- Stage 4 moves the factories of `apiFixtures.ts` that stories use to
  `ui/src/fixtures/<domain>.ts`, and `apiFixtures.ts` re-exports them, so the
  tests keep their imports. Fixtures never import `apiFixtures.ts`: that file
  is test code (the first Knip pass does not check `src/test/`), and the lint
  scope of section 3 does not cover it.
- Time. Fixtures take every date from one constant `NOW` in
  `ui/src/fixtures/time.ts` (a fixed ISO time, such as
  `2026-11-05T10:00:00Z`), and dates relative to it (`minutesAgo(5)`). A
  story passes the same `NOW` as `now` to a component that takes it. So
  "5 min ago" is the same on each run and in each screenshot.
- Edge sets. Each domain fixture file exports sets for edge states: `empty`
  (`[]`), `one` (one item), `many` (50 items), and `longText` (long names and
  long texts, for truncation and wrapping). Example: `agentRows.many`.
- A value that only one story uses stays in that story file.
- A page mockup: `ui/src/mockups/<page>/<Page>.stories.tsx`, built only from
  view components and fixtures. Its handoff is
  `ui/src/mockups/<page>/handoff.md`. A mockup page can have local view
  parts in `ui/src/mockups/<page>/*.view.tsx` while the real view does not
  exist yet (section 1). The developer moves such a part to
  `ui/src/components/` when the real page uses it.
- A component mockup: a change of one component without a page mockup has
  one place, `ui/src/mockups/<component>/`, with the story
  `ui/src/mockups/<component>/<Component>.stories.tsx`
  (`title: "Mockups/<Component>"`), the local view part when the component
  has no view file, and `ui/src/mockups/<component>/handoff.md`. It has the
  same root `ui/src/mockups/` as the page mockups, so the lint scope, Knip
  and the designer paths are the same.
- Size of a page. A page without a height collapses (`NOTES.md`, the old
  620×1180 frame). A mockup story uses `parameters: { layout: "fullscreen" }`
  and has one story for each supported width, named by the width:
  `Width360`, `Width390`, `Width820`, `Width1024`, `Width1180`. The supported widths
  are the projects of `ui/playwright.responsive.config.ts`: 360, 390, 820,
  1024 and 1180 px; the smallest is 360 px. A story sets its width with
  the decorator `withWidth(width)` (stage 3): a frame of that width and the
  full height of the viewport. Storybook without addons has no viewport
  toolbar. `story:shot` (section 7.1) takes the width of its screenshot
  separately.
- Story titles: a component story uses the default title from its path. A
  mockup uses `title: "Mockups/<Page>"` or `title: "Mockups/<Component>"`.

## 6. Format of `handoff.md`

The model is `ui/design_handoff_console_slice/README.md` and `PROMPT.md`. The
model is a one-time handoff of a whole slice with HTML references. The new
file is per mockup and refers to stories, not to HTML. It is in the language
of the designer's request. It has these sections, in this order:

1. `# Handoff: <page or component>` and one line: date, the key of the
   design task, and story ids, such as `mockups-home--width-1024`. The key
   tells the developer where to ask a question, and the review task refers
   to the handoff by it.
2. `## What changed`: a list of visible changes, each with the story that
   shows it.
3. `## New props`: a table with the columns Component, Prop, Type, Meaning,
   Source of data. The type uses `@/types` names or a narrow type
   (`Pick<AgentRow, "name" | "state">`). The source names the API call or
   the state that gives the value, or "unknown". A designer only proposes a
   prop. The developer owns the final type.
4. `## Changed and removed props`: the same table, for a prop whose type or
   meaning changes and for a prop that is removed. The Meaning column says
   what changes.
5. `## States`: each state with its story id (empty, loading, error, long
   text, many items).
6. `## Behavior`: what happens on click, keyboard, hover, focus. A change of
   behavior is named as a change, as in the model's rule: "keep the current
   behavior and say so".
7. `## Texts`: every interface text that the stories do not show: errors
   from the API, toasts, `aria-label` values, tooltips. In English, as the
   product UI.
8. `## Accessibility`: labels for assistive technology, focus order, focus
   return after a dialog closes, how an error is announced, and the contrast
   of new token pairs.
9. `## Layout at widths`: the layout at each supported width (section 5), at
   minimum at the smallest one, each with the story or the `story:shot`
   width that shows it.
10. `## Tokens and styles`: new or changed tokens in `ui/src/index.css` and
    changed `cva()` variants of primitives, if any. Colors only through
    tokens. A change of a token or a primitive variant changes the whole
    interface, so this section lists every affected story, and the designer
    checks those stories in both themes.
11. `## Acceptance criteria`: numbered criteria that the developer and the
    reviewer check one by one.
12. `## Open questions`: questions for the developer.

After implementation:

- The developer writes each answer under its question in `handoff.md`, in
  the pull request that implements the handoff, and adds a first line
  `Implemented in #<PR>`. So the file does not go out of date while the
  answers stay only in the pull request.
- The mockup stays as long as it compiles. The developer deletes the mockup
  directory, `handoff.md` included, when the story of the real page or
  component shows the same states. Git history keeps the handoff.

`PROMPT.md` of the model is a prompt for an implementation run. A new handoff
does not need one. The developer's process starts from `handoff.md`.

## 7. Rules for the two roles

The rules go into a new section "Designer workflow in Storybook" of
`docs/docs/architecture/web-ui.mdx` (stage 4 adds the style rules, stage 7
adds the role rules). A Storybook MDX page is not used, because it needs the
addon `@storybook/addon-docs`, a new dependency for one page. The designer
agent tariboy-designer reviews the designer rules before stage 4 is accepted.

### 7.1 Designer

Style rules. All rules of `ui/.design-sync/conventions.md` move here, except
the ones about the bundle itself (the items "Where the truth is" and "The
old caveat" below say which). The "Setup" rules of `conventions.md`
(`TooltipProvider`, the `.dark` class, the contexts, a matched route) are in
the component notes below and in section 9:

- Character: Tariboy UI is shadcn/ui on Tailwind v4, achromatic metal
  neutrals (hue 250, chroma <= .008) with a single blue accent, Geist
  Variable. Designs read as a control surface for dense operator tooling
  (agent consoles, task trees, image and host tables), not a marketing page.
- Colors are always semantic tokens, never hex and never a Tailwind color
  scale. This makes light and dark both work. The policy test enforces it.
  The token table moves without change: surfaces
  `bg-background bg-card bg-popover bg-muted bg-accent bg-secondary`; text
  `text-foreground text-muted-foreground text-card-foreground
  text-primary-foreground`; emphasis `bg-primary` with
  `text-primary-foreground`; danger `text-destructive`, `bg-destructive/10`;
  lines `border border-border border-t border-b border-input`; focus
  `ring-ring/50 focus-visible:border-ring`.
- CSS custom properties for inline styles and for anything the utilities do
  not cover: `--background --foreground --card --popover --primary
  --secondary --muted --accent --destructive --border --input --ring
  --radius`, each with a `-foreground` partner where it carries text, plus
  `--sidebar*` and `--chart-1…5`.
- `--background` is a window-wide gradient, and `background-color` and
  `color-mix()` drop a gradient value. Use `--background-base` in an inline
  `background-color` and in `color-mix(…, var(--background-base))`.
  `bg-background` resolves to it; only the window itself paints
  `--background`.
- Status is a system, not a color choice. `--status-running` is in
  progress; `--status-failed` is failed or out of budget; `--primary` is
  waiting on the customer; `--status-queued`, `--status-done` and
  `--status-stopped` are quiet text. A fill is only for what is alive and
  what needs a person: `text-status-running` with `bg-status-running/13`,
  `text-status-failed` with `bg-status-failed/12`, `text-primary` with
  `bg-primary/12`. Open, done and cancelled get `text-muted-foreground` and
  no fill.
- Elevation is two levels, never three: `--raise` (a selected row, the
  active segment of a segmented control) and `--lift` (the content island,
  menus). The island is `bg-card`, `rounded-[var(--panel-radius)]`,
  `shadow-[var(--lift)]`, and it sits on a `--background` chrome that has no
  borders.
- Typography: body `text-sm`; secondary meta `text-xs`; headings
  `text-base` or `text-lg` with `font-medium` or `font-semibold`. Task keys,
  image refs, digests, paths and terminal output are `font-mono`.
  Identifiers, durations and timestamps are `font-mono` with `tabular-nums`,
  so they line up down a column.
- Controls, spacing and radius: buttons `h-8` (`h-7` sm, `h-6` xs), inputs
  `h-8`. Spacing is `gap-1`, `gap-2`, `gap-3` and `p-3`, `p-4`. Radius comes
  from `--radius: 0.625rem` through `rounded-md`, `rounded-lg`,
  `rounded-xl`. Console surfaces are denser: rows 30px, buttons 24/26/28px,
  pills 17/19/20px, radii 5/6/7/8/9/12/14px.
- Icons are `lucide-react`, 16px at the default control size and 14px at
  sm, never emoji.
- Dark mode is the class `.dark` on an ancestor (`<html>` in the app).
  `ThemeProvider` sets it, and `ThemeToggle` is the titlebar control.
- The idiomatic example moves as it is:

  ```jsx
  <Card size="sm">
    <CardHeader>
      <CardTitle>worker:v2</CardTitle>
      <CardDescription>Built 2026-09-11 · sha256:9ac71e3f52</CardDescription>
      <CardAction>
        <Badge variant="secondary">Pending</Badge>
      </CardAction>
    </CardHeader>
    <CardContent>
      <div className="flex items-center justify-between gap-2 text-sm">
        <span className="font-mono text-muted-foreground">builder</span>
        <div className="flex gap-2">
          <Button size="sm" variant="outline">
            Export
          </Button>
          <Button size="sm" variant="destructive">
            Remove
          </Button>
        </div>
      </div>
    </CardContent>
  </Card>
  ```

- Where the truth is: the tokens are in `ui/src/index.css`; the API of a
  component is its props type in its source file. (The old text pointed to
  `_ds/<folder>/styles.css`, `<Name>.d.ts` and `<Name>.prompt.md` of the
  bundle. They do not exist in Storybook.)
- The old caveat "the bundle ships only the utilities the app uses; use an
  inline style with `var(--token)`" does not move. Storybook compiles
  `src/index.css` with Tailwind over the same `src` sources, so every
  utility works in a story.
- Use realistic product content (section 5).

Work rules:

- Work only in the designer paths of section 1.
- Never import `@/lib/api`, `@tanstack/react-query` or call `fetch` in view
  files, stories, mockups and fixtures. Lint enforces this (section 3). The
  other designer paths (`ui/src/index.css`, the primitives) have no data
  access to forbid.
- Take types from `@/types`. A view takes the narrowest type it needs:
  `Pick<…>` or its own `XProps`.
- A component without a view file: copy its markup into a local view part
  `ui/src/mockups/<page>/<Name>.view.tsx` (or
  `ui/src/mockups/<component>/<Name>.view.tsx`), change it there, show it
  in the mockup story, and ask in `handoff.md` to split the component. Do
  not edit the container.
- A change of a token or a primitive variant lists every affected story in
  `## Tokens and styles` of the handoff. Check those stories in both themes.
- See a story. Run
  `cd ui && npm run story:shot -- <story-id> <light|dark> <width>`. It builds
  `storybook-static` if there is no build (or with `--rebuild`), serves it
  on a loopback port, and writes a PNG of
  `iframe.html?id=<story-id>&viewMode=story&globals=theme:<theme>`. It prints
  the path of the PNG, `ui/.story-shots/<story-id>.<theme>.<width>.png`.
  Check each changed story in `light` and `dark`, at the smallest supported
  width (360 px) and at 1024 px. The story id is in the URL of the story and
  in `storybook-static/index.json`.
- Hand off a change:
  1. Work in a branch of your own worktree.
  2. Before you hand off, run `make designer-check`. It runs `tsc -b`,
     oxlint on the files changed against `origin/main`,
     `viewLintPolicy.test.ts` and `build-storybook`, under the host check
     slot.
  3. Give the branch and `handoff.md` to the developer through the lead's
     queue. The developer opens the pull request.
- Propose a new prop in `handoff.md`; do not add it to a container.
- Component observations from `ui/.design-sync/NOTES.md` that move with the
  rules:
  - `Tooltip` needs `TooltipProvider`. The global decorator gives it.
  - Radix `open` and `defaultOpen` render overlay content in the story.
    `defaultOpen` works on Dialog, AlertDialog, Sheet, DropdownMenu and
    Collapsible. `ContextMenu` has no `open`: open it with a real
    `contextmenu` event from a mount effect.
  - Overlays need real content behind them, or the backdrop covers an empty
    canvas.
  - Small components need a size: Input and Textarea a labelled field
    320–460px wide, Badge a flex row, a vertical `Separator` an explicit
    height, `ScrollArea` a fixed height with content that overflows.
  - `Switch` uses `defaultChecked`. `checked` without `onCheckedChange` makes
    Radix warn.
  - `CommandList` is `max-h-64`: keep a palette to about 5 items.
    `CommandEmpty` renders with `shouldFilter={false}`.
  - Many components that look self-fetching have injection seams:
    `ShellScriptEditor` takes `load` and `save`, `FileBrowser` takes a
    `FileBrowserApi`, `TuiScreen` takes a `controller`, `PathAutocomplete` is
    controlled. Read the props type first. Two seams are missing, and they
    are requests to the developer, not changes of this project:
    `UpdateBanner` needs an exported context or a snapshot prop, and
    `IterationAuditLog` needs an optional `events` prop.
  - A component that takes its color from data needs that data, or the story
    shows the wrong color: `.agent-header` reads `--agent-hue`, and without it
    the strip is pink.
  - `Button variant="destructive"` is soft (`bg-destructive/10
    text-destructive`) on purpose.
  - Drive a state that has no prop through the DOM: `.click()` opens Radix
    Dialog, AlertDialog, Sheet and Collapsible. Radix `Select` needs a
    `pointerdown` PointerEvent. Inputs take the native value setter and a
    bubbling `input` event.
  - Components that cannot have a meaningful static story: `UpdateBanner`
    (it renders only after a Tauri update snapshot), and
    `CustomerQuestionNotifications` (headless, and it never goes into a
    story: section 9). `IterationAuditLog` shows only "No events." without a
    daemon.
  - A page story without a daemon shows the genuine empty or error state.
    That is real content, not a placeholder.
  - `Separator`, `SheetFooter`, `AlertDialogMedia`, `DropdownMenuLabel`,
    `DropdownMenuSeparator` and `AlertDialogContent size="sm"` are not used
    in the app. Their stories are plausible compositions; re-ground them
    when real usage appears.
  - Typing into a field that your own click revealed needs two timer ticks
    (about 120 ms for the click, 260 ms for the typing).
  - A host without an emoji font shows emoji in component source
    (`ProxyCallRow` markers) as empty boxes in a `story:shot` screenshot.
    This is a screenshot artifact, not a bug of the component.
  - Open product findings (not changed by this project): `FileBrowser` uses
    `prose` classes without `@tailwindcss/typography`; the `--agent-hue`
    fallback of `.agent-header` is pink, not neutral.
  - The NOTES sections about the sync tool itself (bundle, entry, capture
    server, token classification, re-sync) do not move.

### 7.2 Developer

- Own the props type of every view file. A change of a props type changes
  the stories in the same commit, and `tsc -b` fails until they compile.
- Keep data hooks, `@/lib/api`, react-query, router hooks, state and handlers
  in containers and pages. A view takes props and callbacks and returns JSX.
- Import types from `@/types` in new code. `@/lib/api` still re-exports them.
- Move a component to the view layer one at a time: split data and markup,
  rename the markup file to `*.view.tsx`, fix lint and the policy test for
  it, and add or move its story.
- Change primitives in `ui/src/components/ui/` only for accessibility and
  behavior, and tell the designer.
- Implement a `handoff.md`: open the pull request from the designer's
  branch, answer each open question under it in `handoff.md` with the first
  line `Implemented in #<PR>` (section 6), and keep the final type in the
  view file. When the handoff asks to split a component, split it and move
  the local view part of the mockup into the new view file.
- When a props change breaks a story, fix the story in the same commit and
  mention `@agent:tariboy-designer` on the task of the change.
- Keep the Storybook setup (`ui/.storybook/`, `ui/src/storybook/`,
  `ui/vite.storybook.config.ts`, `ui/scripts/story-shot.mjs`) and the
  checks of section 8 green.

## 8. Integration into `make frontend-check`

Current steps: `ui-fmt-check`, `ui-api-types`, `ui-typecheck`
(`npx tsc -b --checkers 1`), `ui-lint`, `ui-dead-code` (Knip), the memory gate
and `ui-test`, `ui-branding`, `ui-bundle-budget`, `docs`.

| Area | Change |
| --- | --- |
| Packages | devDependencies `storybook` and `@storybook/react-vite`, version 10.6.5 (`npm view @storybook/react-vite version` on 2026-11-05). Peer dependencies: `vite ^5 \|\| ^6 \|\| ^7 \|\| ^8`, `react ^16.8 … ^19`, `typescript >= 4.9.x`. `ui/package.json` has `vite ^8.0.12` and `react ^19.2.6`. No addon |
| Scripts | `"storybook": "storybook dev -p 6006 --host 127.0.0.1"`, `"build-storybook": "storybook build"`, `"story:shot": "node scripts/story-shot.mjs"` (section 7.1, stage 3) |
| Vite configuration | `ui/vite.storybook.config.ts`, with no `server.proxy` (section 9.1). `ui/tsconfig.node.json` type-checks it with the other Vite files |
| `make designer-check` | A new Makefile target for the designer (section 7.1, stage 5): `tsc -b`, oxlint on the files changed against `origin/main`, `viewLintPolicy.test.ts` with `TARIBOY_VITEST_MAX_WORKERS=2`, `build-storybook`, under `CHECK_SLOT`. It is not a step of `frontend-check`, which runs all of these already |
| New step | `ui-storybook` after `ui-test`: `cd ui && npm run build-storybook -- --output-dir "$TMPDIR/storybook-static" --quiet`. `$TMPDIR` is the run directory of the check, which the step runner removes |
| Story types | `ui-typecheck` already covers stories, mockups and fixtures: `tsconfig.app.json` includes `src`. `ui/tsconfig.storybook.json` (extends `tsconfig.app.json`, includes `.storybook`) is added to the `references` of `ui/tsconfig.json`. Stage 4 removes `tsconfig.design-sync.json` from them |
| Lint | `oxlint .` covers `.storybook/` and stories. The six thresholds apply to stories. The 55 previews have at most 352 lines (`AuditRow.tsx`), under `max-lines` 400. `storybook-static` and `.story-shots` go to `ignorePatterns` |
| Format | `fmt` and `fmt:check` get `.storybook` in place of `.design-sync` |
| Knip | Knip 6.40.0 has a Storybook plugin (`ui/node_modules/knip/dist/plugins/storybook/index.js`). It turns on when a dependency matches `^@storybook/`, reads `.storybook/main.ts`, and makes `.storybook/{manager,preview}.{ts,tsx}` and `**/*.stories.tsx` entries in the second pass. The first pass (`--production`) does not use them, so `project` gets `!src/**/*.stories.tsx!`, `!src/mockups/**!`, `!src/fixtures/**!`, `!src/storybook/**!`, the same form as `!src/test/**!` |
| `bundle:budget` | No change. The app entry does not import stories, mockups, fixtures or Storybook, and Vite builds only what the entry imports. Stage 3 runs `npm run bundle:budget` before and after and expects the same three numbers |
| CSS | Tailwind already scans `src`, and today it also scans the previews. Stories in `src` keep that state. A class used only in a story gets into the app CSS. The JS budget does not count CSS |
| `ui-test` | Vitest does not collect `*.stories.tsx` (it collects `*.test.*`). New tests: section 3 and section 4 |
| `.gitignore` | `storybook-static/`, `.story-shots/` |

Time and memory. Stage 1 cannot run `build-storybook` because Storybook is
not installed yet. The estimate uses the production build of the app, which
uses the same Vite, plugins and sources:

```bash
cd ui
flock -o /tmp/tariboy-check.lock /usr/bin/time -v npx vite build -c vite.desktop.config.ts --outDir <scratch dir> --emptyOutDir
```

Result on this host (6 CPUs, 5924 MB RAM, 3661 MB available before the run):
`built in 2.91s`, `Elapsed (wall clock) time: 0:05.20`,
`Maximum resident set size (kbytes): 665388` (650 MB).

The Storybook build also bundles the manager UI and one entry for each story
file (52 or more). The estimate is 2 to 4 times the app build: 15 to 30
seconds and 1.0 to 1.5 GB peak. The step runs after `ui-test`, so it does not
compete with the Vitest workers. Stage 3 measures the real value with the same
`/usr/bin/time -v` command. If the peak is over 1.5 GB, stage 3 sets
`NODE_OPTIONS=--max-old-space-size=1536` for the step and records the value in
`docs/docs/development.mdx`.

## 9. Migration of the 55 previews

Preview format now: named exports of state functions
(`export const States = () => …`), components imported from `"tariboy-ui"`
(the generated `.design-sync/entry.tsx`), and backdrop wrappers in the file.
A story file has a default `meta` (`component`, `decorators`, `parameters`)
and one named story for each preview export, with the same name. The
component map comes from `componentSrcMap` in `ui/.design-sync/config.json`.

Decorators, all in `ui/src/storybook/decorators.tsx` (developer code, outside
the lint scope of section 3):

- Global, in `ui/.storybook/preview.tsx`: the theme (class `.dark` on
  `<html>`, toolbar switch light and dark), `TooltipProvider`, and
  `MemoryRouter`.
- `withQueryClient`: a new `createQueryClient()` for each story, and
  `QueryClientProvider`. It replaces the client that the `TasksWorkspace`
  and `TaskDetail` previews create in the file. Turned on with
  `parameters: { tariboy: { query: true } }`.
- `withDaemon`: `DaemonProvider` with a registry that points to an
  unreachable host (see 9.1). Turned on with
  `parameters: { tariboy: { daemon: true } }`.
- `withWidth(width)`: a frame of that width and the full viewport height,
  for the page mockups and page stories (section 5).
- `withRoute(path, pattern)`: `MemoryRouter` with `initialEntries` and a
  matched `Route`, for components that read `useParams()`. Turned on with
  `parameters: { tariboy: { route: { path, pattern } } }`.
- `withSurface(kind, width)`: a backdrop: `sidebar` (the `sidebar(...)`
  wrapper of `AgentRow`), `panel`, `card`, `row`. It replaces the local
  wrapper functions such as `sidebar`, `row`, `panel`, `field`, `frame`.

So a story imports no `@tanstack/react-query` and no `@/lib/api`. A preview
type such as `ComponentProps<typeof TaskDetail>["detail"]` stays, or changes
to the `@/types` name.

### 9.1 Storybook never reaches a daemon

This is a safety requirement. A story must be safe to open. Today
`ui/vite.config.ts` (lines 19–21) proxies `/api` to
`http://127.0.0.1:9990`, the live listener, and `@storybook/react-vite`
reads `vite.config.ts` by default. `DaemonProvider` without a registry uses
`baseURL: ""` (`unresolvedDaemon` in `ui/src/lib/daemons.ts`, line 47),
which is the relative `/api`. So a page story with a plain `DaemonProvider`
could read or change the live daemon.

The mechanism:

- Storybook has its own Vite configuration file,
  `ui/vite.storybook.config.ts`, set with the builder option
  `viteConfigPath` in `ui/.storybook/main.ts`. The file has the plugins
  `react()` and `tailwindcss()`, the alias `@` → `src` and `server.fs.allow`
  from `vite.fsAllow`. It has no `server.proxy`. The other option was a
  `viteFinal` that deletes `server.proxy` from the inherited
  `vite.config.ts`. It was rejected: a deletion removes only the key it
  names, so a later server option in `vite.config.ts` (a second proxy path,
  a websocket proxy) gets into Storybook without a review. A separate file
  has nothing to remove, and it follows the repository pattern of one Vite
  file for each target (`vite.desktop.config.ts`, `vite.store.config.ts`).
- `withDaemon` writes a registry with one host on the reserved top-level
  domain `.invalid` (RFC 2606): `baseURL: "http://storybook.invalid"`, id
  `storybook`, and makes it active. A name under `.invalid` never resolves,
  so no request reaches any daemon. The page shows its genuine empty or error
  state. The desktop cleartext check of `daemons.ts` (line 108) runs only in
  the Tauri host, not in Storybook.
- `withDaemon` saves `tariboy_daemons` and `tariboy_active_daemon` before
  the story and restores both after each story (a `useLayoutEffect` cleanup,
  or the removal of a key that did not exist). Storybook switches stories in
  one iframe, so without it a value that one story writes goes into the next
  story.
- `CustomerQuestionNotifications` is not in any story or mockup. It opens a
  tasks websocket that does not close (`NOTES.md`, "Page-level surfaces").
  Its context has a usable default, so the pages that read it render
  without it. Its preview `HeadlessPassthrough` is not migrated: it is
  headless and shows only its child.

Tests of stage 3, RED before the code:

- `ui/src/test/storybookConfig.test.ts`: the builder option of
  `.storybook/main.ts` names `vite.storybook.config.ts`; the Vite
  configuration that it loads (with `loadConfigFromFile` of `vite`, then
  `viteFinal` of `main.ts` if it exists) has no `server.proxy`, and its JSON
  text contains neither `/api` as a proxy key nor `127.0.0.1:9990`.
- `ui/src/test/storybookDecorators.test.tsx`: `withDaemon` sets the active
  host to a `baseURL` that ends in `.invalid`, and after the unmount both
  `localStorage` keys have their values from before the story (a set value
  and a missing key).
- `ui/src/test/storybookSafety.test.ts`: no file of
  `src/**/*.stories.tsx` and `src/mockups/**` imports
  `CustomerQuestionNotifications`: a case-sensitive search for an import
  from a path that ends in `/CustomerQuestionNotifications`. The context
  module `customerQuestionNotificationsContext.ts` does not match.

Manual check of stage 3: open a page story in `npm run storybook`, and in
the browser network panel (or a Playwright `page.on("request")` list) see no
request to `127.0.0.1:9990` and no request to a relative `/api`.

### 9.2 Order and map

Order of migration, one commit for each group. 54 of the 55 previews become
stories; `CustomerQuestionNotifications` does not (9.1):

1. Primitives in `ui/src/components/ui/` (story next to the primitive file,
   for example `ui/src/components/ui/button.stories.tsx`): AlertDialog,
   Badge, Button, Card, Collapsible, Command, ContextMenu, Dialog,
   DropdownMenu, Input, Label, PriorityTag, ScrollArea, Segmented, Select,
   Separator, Sheet, StatusDot, StatusPill, Switch, Tabs, Textarea, Toaster,
   Tooltip (24). PriorityTag, StatusDot and StatusPill are all in
   `status.tsx`, so they are one file `status.stories.tsx` with three groups
   of stories (`title` per export is not needed: the story names keep the
   prefix, such as `PriorityTagRanks`).
2. Product components in `ui/src/components/`: AgentControls, AgentRow,
   AuditExportActions, AuditRow, ComboField, DaemonBanner,
   EditablePresetCombobox, FileBrowser, FullAuditLog, GoalHelp, GroupWizard,
   HostStatus, ImageCombobox (`AgentFormFields.tsx`), ImageLayout,
   IterationAuditLog, PathAutocomplete, ProxyCallRow, SendFilesButton,
   ShellScriptEditor, SupportBundle, ThemeToggle, TuiScreen, AppSettings
   (`DesktopUpdates.tsx`) (23).
3. Pages and containers: TasksWorkspace, TaskDetail (`src/pages/tasks/`),
   SettingsPage (`src/pages/settings/`), StoresPage (`src/pages/`),
   TerminalsPage (`src/pages/terminals/`), LibraryImagesPage
   (`src/pages/images/`), AgentWorkspace (`src/pages/agents/`) (7).

The table of all 55:

| Preview | Component source | Story | Decorators | Notes |
| --- | --- | --- | --- | --- |
| AgentControls | `src/components/AgentControls.tsx` | `src/components/AgentControls.stories.tsx` | – | |
| AgentRow | `src/components/AgentRow.tsx` | `src/components/AgentRow.stories.tsx` | `withSurface("sidebar", 260)` | becomes `AgentRow.view.tsx` in stage 5 |
| AgentWorkspace | `src/pages/agents/AgentWorkspace.tsx` | `src/pages/agents/AgentWorkspace.stories.tsx` | daemon, route | `useParams()` needs a matched route |
| AlertDialog | `src/components/ui/alert-dialog.tsx` | `src/components/ui/alert-dialog.stories.tsx` | – | `defaultOpen` |
| AppSettings | `src/components/DesktopUpdates.tsx` | `src/components/DesktopUpdates.stories.tsx` | router (global) | |
| AuditExportActions | `src/components/AuditExportActions.tsx` | `src/components/AuditExportActions.stories.tsx` | – | local `mono` style becomes a class |
| AuditRow | `src/components/AuditRow.tsx` | `src/components/AuditRow.stories.tsx` | `withSurface("panel")` | 352 lines, local event builders stay in the story |
| Badge | `src/components/ui/badge.tsx` | `src/components/ui/badge.stories.tsx` | `withSurface("row")` | |
| Button | `src/components/ui/button.tsx` | `src/components/ui/button.stories.tsx` | `withSurface("row")` | |
| Card | `src/components/ui/card.tsx` | `src/components/ui/card.stories.tsx` | – | |
| Collapsible | `src/components/ui/collapsible.tsx` | `src/components/ui/collapsible.stories.tsx` | – | |
| ComboField | `src/components/ComboField.tsx` | `src/components/ComboField.stories.tsx` | – | |
| Command | `src/components/ui/command.tsx` | `src/components/ui/command.stories.tsx` | – | about 5 items |
| ContextMenu | `src/components/ui/context-menu.tsx` | `src/components/ui/context-menu.stories.tsx` | – | opened by a `contextmenu` event |
| CustomerQuestionNotifications | `src/components/CustomerQuestionNotifications.tsx` | none | – | not migrated: headless, opens a tasks websocket (9.1) |
| DaemonBanner | `src/components/DaemonBanner.tsx` | `src/components/DaemonBanner.stories.tsx` | – | |
| Dialog | `src/components/ui/dialog.tsx` | `src/components/ui/dialog.stories.tsx` | – | |
| DropdownMenu | `src/components/ui/dropdown-menu.tsx` | `src/components/ui/dropdown-menu.stories.tsx` | – | |
| EditablePresetCombobox | `src/components/EditablePresetCombobox.tsx` | `src/components/EditablePresetCombobox.stories.tsx` | – | |
| FileBrowser | `src/components/FileBrowser.tsx` | `src/components/FileBrowser.stories.tsx` | – | `FileBrowserApi` fake stays in the story |
| FullAuditLog | `src/components/FullAuditLog.tsx` | `src/components/FullAuditLog.stories.tsx` | – | |
| GoalHelp | `src/components/GoalHelp.tsx` | `src/components/GoalHelp.stories.tsx` | – | |
| GroupWizard | `src/components/GroupWizard.tsx` | `src/components/GroupWizard.stories.tsx` | – | |
| HostStatus | `src/components/HostStatus.tsx` | `src/components/HostStatus.stories.tsx` | – | |
| ImageCombobox | `src/components/AgentFormFields.tsx` | `src/components/AgentFormFields.stories.tsx` | – | story names keep `ImageCombobox` |
| ImageLayout | `src/components/ImageLayout.tsx` | `src/components/ImageLayout.stories.tsx` | daemon, route | |
| Input | `src/components/ui/input.tsx` | `src/components/ui/input.stories.tsx` | – | |
| IterationAuditLog | `src/components/IterationAuditLog.tsx` | `src/components/IterationAuditLog.stories.tsx` | – | shows "No events." |
| Label | `src/components/ui/label.tsx` | `src/components/ui/label.stories.tsx` | – | |
| LibraryImagesPage | `src/pages/images/LibraryImagesPage.tsx` | `src/pages/images/LibraryImagesPage.stories.tsx` | daemon, route | |
| PathAutocomplete | `src/components/PathAutocomplete.tsx` | `src/components/PathAutocomplete.stories.tsx` | – | |
| PriorityTag | `src/components/ui/status.tsx` | `src/components/ui/status.stories.tsx` | – | |
| ProxyCallRow | `src/components/ProxyCallRow.tsx` | `src/components/ProxyCallRow.stories.tsx` | – | |
| ScrollArea | `src/components/ui/scroll-area.tsx` | `src/components/ui/scroll-area.stories.tsx` | – | fixed height |
| Segmented | `src/components/ui/segmented.tsx` | `src/components/ui/segmented.stories.tsx` | – | |
| Select | `src/components/ui/select.tsx` | `src/components/ui/select.stories.tsx` | – | |
| SendFilesButton | `src/components/SendFilesButton.tsx` | `src/components/SendFilesButton.stories.tsx` | `withSurface("panel")` | the toolbar under a `TuiScreen`, see below |
| Separator | `src/components/ui/separator.tsx` | `src/components/ui/separator.stories.tsx` | – | vertical needs a height |
| SettingsPage | `src/pages/settings/SettingsPage.tsx` | `src/pages/settings/SettingsPage.stories.tsx` | daemon, route | |
| Sheet | `src/components/ui/sheet.tsx` | `src/components/ui/sheet.stories.tsx` | – | |
| ShellScriptEditor | `src/components/ShellScriptEditor.tsx` | `src/components/ShellScriptEditor.stories.tsx` | – | |
| StatusDot | `src/components/ui/status.tsx` | `src/components/ui/status.stories.tsx` | – | |
| StatusPill | `src/components/ui/status.tsx` | `src/components/ui/status.stories.tsx` | – | |
| StoresPage | `src/pages/StoresPage.tsx` | `src/pages/StoresPage.stories.tsx` | daemon, route | |
| SupportBundle | `src/components/SupportBundle.tsx` | `src/components/SupportBundle.stories.tsx` | – | |
| Switch | `src/components/ui/switch.tsx` | `src/components/ui/switch.stories.tsx` | – | `defaultChecked` |
| Tabs | `src/components/ui/tabs.tsx` | `src/components/ui/tabs.stories.tsx` | – | |
| TaskDetail | `src/pages/tasks/TaskDetail.tsx` | `src/pages/tasks/TaskDetail.stories.tsx` | daemon, query | the panel docks to the viewport |
| TasksWorkspace | `src/pages/tasks/TasksWorkspace.tsx` | `src/pages/tasks/TasksWorkspace.stories.tsx` | daemon, query | |
| TerminalsPage | `src/pages/terminals/TerminalsPage.tsx` | `src/pages/terminals/TerminalsPage.stories.tsx` | daemon, route | `SidebarStateProvider` stays in the story |
| Textarea | `src/components/ui/textarea.tsx` | `src/components/ui/textarea.stories.tsx` | – | |
| ThemeToggle | `src/components/ThemeToggle.tsx` | `src/components/ThemeToggle.stories.tsx` | – | |
| Toaster | `src/components/ui/sonner.tsx` | `src/components/ui/sonner.stories.tsx` | – | imports `toast` from `sonner` directly |
| Tooltip | `src/components/ui/tooltip.tsx` | `src/components/ui/tooltip.stories.tsx` | – | `TooltipProvider` is global |
| TuiScreen | `src/components/TuiScreen.tsx` | `src/components/TuiScreen.stories.tsx` | – | `controller` fakes stay in the story |

Notes:

- `Toaster`: the old preview imported `toast` from the bundle to share one
  sonner singleton. In Storybook the story and the component use the same
  `node_modules/sonner`, so the story imports `toast` from `sonner`.
- `SendFilesButton` `TerminalToolbar`: the fake terminal backdrop
  (`var(--color-zinc-950, #09090b)`, `#d4d4d8`) is not kept. The story
  renders the toolbar under a real `TuiScreen` with a fake `controller`, as
  in the `TuiScreen` stories, so the button keeps its context above the
  terminal without raw colors.
- Page stories (the 7 of group 3) use `withDaemon` (9.1),
  `parameters: { layout: "fullscreen" }` and `withWidth` (section 5), so
  they show the real empty or error state at a real page size.
- `AgentNameContext`, `AgentStatusContext` and
  `CustomerQuestionNotificationsContext` are imported from their source
  modules. Storybook builds one module graph, so the old rule "use the
  bundle's context instance" does not apply. The context is in its own
  module `src/components/customerQuestionNotificationsContext.ts`, so a
  story can import it without the component.
- The migration test (stage 4) compares each preview's export names with the
  story's export names, so no state is lost. It skips
  `CustomerQuestionNotifications` by name.

## 10. References to Claude Design

Command (repository root, task worktree):

```bash
rg -i -n 'design-sync|ds-bundle|tariboy-ui"' --hidden -g '!node_modules' | grep -v -E '^ui/(\.design-sync|ds-bundle)/'
```

Result: 45 lines in 14 files. In the main checkout, where `.git` is a
directory, the same `rg` also finds 12 lines in `.git/packed-refs` (1),
`.git/info/refs` (2) and `.git/logs/HEAD` (9). `ui/ds-bundle/` is not
tracked: `ui/.gitignore` ignores it, and the worktree does not have it. After
this pull request, the two files of this pull request also match.

| File | Line | Decision | Stage |
| --- | --- | --- | --- |
| `docs/docs/development.mdx` | 597 | Replace: `fmt` paths name `ui/.storybook`, not `ui/.design-sync` | 4 |
| `docs/docs/development.mdx` | 625 | Replace: one override kind (tests) after stage 4; stage 5 adds the view-layer override | 4, 5 |
| `docs/docs/development.mdx` | 685 | Delete (paragraph about the `.design-sync` override) | 4 |
| `docs/docs/development.mdx` | 686 | Delete (same paragraph) | 4 |
| `docs/docs/development.mdx` | 741 | Replace: stories, mockups, fixtures and `src/storybook` are not entry points of the first Knip pass | 4 |
| `docs/docs/development.mdx` | 742 | Replace: "only a test or a story imports" | 4 |
| `docs/docs/development.mdx` | 744 | Replace: the second pass checks stories and `.storybook/` | 4 |
| `docs/docs/development.mdx` | 745 | Replace (same sentence) | 4 |
| `docs/docs/development.mdx` | 753 | Replace: `entry` has only `tests/**`; the Storybook plugin finds the stories | 4 |
| `docs/docs/development.mdx` | 782 | Replace: the kit stays complete for the Storybook catalog | 4 (text), 7 (full section) |
| `docs/docs/development.mdx` | 786 | Replace: the paragraph describes the Storybook checks (`tsc -b`, `ui-storybook`) | 4 (text), 7 (full section) |
| `docs/docs/development.mdx` | 787 | Replace (same paragraph) | 4 |
| `docs/docs/development.mdx` | 788 | Replace (same paragraph) | 4 |
| `docs/docs/development.mdx` | 792 | Delete (`make-entry.mjs` check) | 4 |
| `docs/docs/development.mdx` | 795 | Delete (same) | 4 |
| `ui/design_handoff_console_slice/README.md` | 144 | Replace: "check the slice in Storybook" in place of `/design-sync` | 4 |
| `ui/.gitignore` | 4 | Delete `ds-bundle/` | 4 |
| `ui/.gitignore` | 5 | Delete `.design-sync/.cache/` | 4 |
| `ui/.gitignore` | 6 | Delete `.design-sync/learnings/` | 4 |
| `ui/.gitignore` | 7 | Delete `.design-sync/node_modules` | 4 |
| `ui/knip.json` | 3 | Delete `.design-sync/**/*.{ts,tsx,mjs}` from `entry` | 4 |
| `ui/knip.json` | 9 | Delete `.design-sync/**/*.{ts,tsx,mjs}` from `project` | 4 |
| `ui/.oxlintrc.json` | 104 | Delete the whole `.design-sync/**` override (lines 102–114) | 4 |
| `ui/package.json` | 2 | Keep `"name": "tariboy-ui"`: the npm package name of `ui/`, not a Claude Design reference | – |
| `ui/package.json` | 17 | Replace: `oxfmt src store tests .storybook` | 4 |
| `ui/package.json` | 18 | Replace: `oxfmt --check src store tests .storybook` | 4 |
| `ui/package-lock.json` | 2 | Keep: package name | – |
| `ui/package-lock.json` | 8 | Keep: package name | – |
| `ui/scripts/complexity-report.mjs` | 22 | Replace the comment: "(test files)" | 4 |
| `ui/src/index.css` | 6 | Delete the comment and the two `@source not` lines (lines 6–12) | 4 |
| `ui/src/index.css` | 11 | Delete (same block) | 4 |
| `ui/src/index.css` | 12 | Delete (same block) | 4 |
| `ui/src/test/designQueryProvider.test.ts` | 2 | Delete the file. Stage 3's decorator test covers one query client for each story | 4 |
| `ui/src/test/designQueryProvider.test.ts` | 3 | Delete (same file) | 4 |
| `ui/src/test/designQueryProvider.test.ts` | 4 | Delete (same file) | 4 |
| `ui/src/test/designQueryProvider.test.ts` | 5 | Delete (same file) | 4 |
| `ui/src/test/designSyncCatalog.test.ts` | 8 | Delete the file | 4 |
| `ui/src/test/designSyncCatalog.test.ts` | 13 | Delete (same file) | 4 |
| `ui/src/test/lintPolicy.test.ts` | 32 | Replace: remove `designSyncOverride` | 4 |
| `ui/src/test/lintPolicy.test.ts` | 45 | Replace: scan `["src", "store", ".storybook"]` | 4 |
| `ui/src/test/lintPolicy.test.ts` | 57 | Replace: "overrides rules only for test files" (stage 5: "and the view layer") | 4, 5 |
| `ui/tsconfig.design-sync.json` | 4 | Delete the file | 4 |
| `ui/tsconfig.design-sync.json` | 5 | Delete (same file) | 4 |
| `ui/tsconfig.design-sync.json` | 7 | Delete (same file) | 4 |
| `ui/tsconfig.json` | 6 | Delete. Stage 3 adds `{ "path": "./tsconfig.storybook.json" }` next to it, so `tsc -b` checks the previews until stage 4 removes them | 4 |
| `.git/packed-refs`, `.git/info/refs`, `.git/logs/HEAD` (main checkout) | 12 lines | Keep: branch names and the reflog, not files of the tree | – |
| `docs/superpowers/specs/2026-11-05-storybook-design-workflow-design.md`, `docs/superpowers/plans/2026-11-05-storybook-design-workflow.md` | all | Keep: design history | – |

Two more references that the `rg` pattern does not find:

| File | Line | Decision | Stage |
| --- | --- | --- | --- |
| `ui/.gitignore` | 3 | Delete `.ds-sync/` (an old Claude Design cache directory) | 4 |
| `ui/.design-sync/`, `ui/ds-bundle/` | – | Delete the directories (`ds-bundle/` is not tracked; delete it from a local checkout by hand) | 4 |

After stage 4, the `rg` command finds only the "Keep" rows.

### 10.1 `make-tokens.mjs`, `annotate-tokens.mjs` and `/* @kind … */`

Command:

```bash
rg -n 'make-tokens|annotate-tokens|@kind' --hidden -g '!node_modules' | grep -v '^ui/.design-sync/'
```

Result: 7 lines in `ui/src/index.css` (21, 133, 152, 156, 157, 202, 203:
`/* @kind font */`, `/* @kind radius */`, `/* @kind shadow */`), line 142 of
`ui/design_handoff_console_slice/README.md` and line 18 of
`ui/design_handoff_console_slice/PROMPT.md`. Inside `ui/.design-sync/`, the
names are in `make-tokens.mjs`, `annotate-tokens.mjs`, `config.json`,
`NOTES.md`, `overrides/css.mjs` and `prepare-css.sh`. No script, test,
Makefile target or Vite plugin outside `ui/.design-sync/` reads the scripts
or the annotations. Tailwind and the browser ignore CSS comments.

Decision: delete both scripts with `ui/.design-sync/` in stage 4, and delete
the seven `/* @kind … */` comments from `ui/src/index.css` in the same
commit. In `ui/design_handoff_console_slice/`, delete item 2 of the README
section "Заодно починить в стилях" (line 142) and change "оба пункта" to
"пункт" in `PROMPT.md` line 18. The handoff stays as the format model, as
decision 7 requires.

## 11. Visual regression screenshots of stories (approach only)

This project does not implement regression screenshots. The next task uses
this approach. It is separate from `story:shot` (section 7.1, stage 3), the
designer's tool to see one story: `story:shot` compares nothing and keeps no
baseline. The regression suite can reuse its static server and URL builder.

- Build Storybook once with `npm run build-storybook` into a temporary
  directory, and serve it with a static server on `127.0.0.1` and a port
  that the OS selects.
- Read `index.json` of the build. It lists every story id. Each story id
  becomes one Playwright test.
- Open `iframe.html?id=<story-id>&viewMode=story` for each story, in two
  projects: light, and dark (the `globals` query parameter of the theme
  toolbar, `&globals=theme:dark`).
- Make the page stable: turn off animations and transitions with a style
  sheet, wait for `document.fonts.ready` (Geist from
  `@fontsource-variable/geist`), and fix the viewport (1024×700) and
  `Date` (fixtures and stories use `NOW` of `ui/src/fixtures/time.ts`,
  section 5).
- Compare with `expect(page).toHaveScreenshot()`. The baselines are in
  `ui/tests/storybook.pw.ts-snapshots/`, made on Linux in the CI image, so
  font rendering is the same.
- Configuration: `ui/playwright.storybook.config.ts`, script
  `test:storybook-visual`. It runs in `make full-check`, not in `make check`,
  like the other browser suites.
- Skip a story with `parameters: { visual: false }` (for example
  `IterationAuditLog`, `UpdateBanner`).

## 12. Differences from decisions 1–7

1. Decision 1 and decision 2 name `AgentRow.tsx` as the view example. By
   section 2 the file becomes `AgentRow.view.tsx` (stage 5), and its story is
   `AgentRow.stories.tsx` next to it. Reason: a view file needs a mark that a
   tool can check.
2. Decision 4 says "alias `@/types`". No new alias is added: the existing
   `@/*` alias resolves `@/types` in TypeScript and in Vite. Reason: a second
   alias for the same path changes nothing.
3. Decision 7 says to delete the references together with
   `.design-sync/` and `ds-bundle/`. Three kinds of `rg` matches stay:
   `"name": "tariboy-ui"` in `ui/package.json` and `ui/package-lock.json`
   (the npm package name), the files of `.git/` (branch names and reflog),
   and these two documents in `docs/superpowers/` (history). Reason: none of
   them is a reference to the Claude Design tool.
4. Decision 5 forbids imports in the view layer. A story of a container
   imports the container, and the container imports `@/lib/api`. The lint
   checks only the direct imports of a file. Reason: decision 5 limits files,
   and the stories of the 7 page previews must render the real pages.
5. Brief, stage 4: "rules from `conventions.md` → a Docs page in Storybook
   and/or `web-ui.mdx`". The rules go only to `web-ui.mdx`. Reason: a Docs
   page needs `@storybook/addon-docs`, a new dependency.
6. The project and the queue: the queue DEV has `AUTO_MERGE` on, but this
   pull request is a draft and is not merged automatically. The customer
   merges it after the review of tariboy-designer and the agreement of the
   user. Reason: the PM forbade the automatic merge for this project
   (comment 6419 on the lead task of stage 1).
7. Decision 7 ("previews → stories") and stage 4 of the brief ("move all
   55 previews") ask to migrate every preview of
   `ui/.design-sync/previews/`. 54 of the 55 become stories.
   `CustomerQuestionNotifications` does not.
   Reason: the customer's safety decision for the review finding O8 (9.1):
   the component opens a tasks websocket that does not close, and it is
   headless, so its story shows nothing of its own. The customer's
   rework of this pull request requires the safety test that forbids it in
   stories, so this difference is the customer's decision.

The review of tariboy-designer (findings O1–O8 and Zh1–Zh12) adds no other
difference. The component mockups of section 5, the local view parts of
section 1, `story:shot` and `make designer-check` are new paths and tools
inside decisions 1–7.
