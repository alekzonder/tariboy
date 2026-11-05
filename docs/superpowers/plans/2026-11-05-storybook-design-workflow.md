# Storybook design workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Claude Design catalog in `ui/.design-sync/` with Storybook stories next to the components, typed against `@/types`, with lint boundaries for the view layer, one pilot view/container split and one page mockup with `handoff.md`.

**Architecture:** `ui/src/types/` holds type-only re-exports of the generated API types. Storybook (`@storybook/react-vite`) builds the stories from `ui/src` with the app's Vite and Tailwind setup. View files (`*.view.tsx`), stories, mockups and fixtures get an oxlint override and a policy test that keep data access out of them.

**Tech Stack:** React 19, Vite 8, Tailwind CSS 4.3.0, TypeScript, Vitest, oxlint 1.81.0, Knip 6.40.0, Storybook 10.6.5 (`storybook`, `@storybook/react-vite`).

**Spec:** `docs/superpowers/specs/2026-11-05-storybook-design-workflow-design.md`

## Global Constraints

- Stages run in order 2, 3, 4, 5, 6, 7. Each stage is its own pull request, and the next stage starts after the user accepts the previous one.
- Each stage works in its own branch and worktree. History is linear: rebase, never merge.
- Run every check, oxlint and Vitest only under `flock -o /tmp/tariboy-check.lock`, one at a time. Vitest runs with `TARIBOY_VITEST_MAX_WORKERS=2`.
- Never start a daemon against the live `~/.tariboy`, `~/.tariboyd` or `127.0.0.1:9990`. `make ui-dev` uses its isolated API.
- No commit or pull request carries a co-author trailer or a mention of AI. No version bump.
- Out of scope: redesign of components, token changes, mass split of components into view and container, backend, OpenAPI and `ui/src/lib/api-types.generated.ts` changes.
- `@/lib/api` keeps re-exporting every type it re-exports today.
- Storybook version: `storybook` and `@storybook/react-vite` at the same exact version, the latest 10.x that `npm view @storybook/react-vite version` prints on the day of stage 3 (10.6.5 on 2026-11-05).
- View-layer glob, the same in oxlint and in the policy test: `src/**/*.view.tsx`, `src/**/*.stories.tsx`, `src/mockups/**`, `src/fixtures/**`.
- Before each commit: `git diff --check` and a read of the whole diff.
- Check of a stage: `make frontend-check` (UI and docs only). `make check` only when a stage touches the `Makefile` step runner or files outside `ui/` and `docs/`.

## Review Focus

- A story that passes a wrong prop must fail `tsc -b`. A story under `src/` outside `tsconfig.app.json` would pass silently. Task 3 pins this with its RED story.
- A component that reads `useParams()` (AgentWorkspace, ImageLayout, LibraryImagesPage) renders empty under a plain `MemoryRouter`. Task 5 checks that each such story mounts a matched route.
- The dark theme: a story that looks right only in light mode. Task 5 and Task 8 include a visual check of both themes.
- The first Knip pass reports stories, fixtures and decorators as unused files when `project` does not exclude them. Task 3 pins this with `npm run knip` after the first story.
- The oxlint override silently stops applying after a rename of the glob or an oxlint update. Task 6 runs oxlint on violation files as a test, not only a config compare.

---

## Stage 2: `@/types`

### Task 1: Type-only directory `ui/src/types/`

**Files:**
- Create: `ui/src/types/{agents,audit,daemon,files,groups,harnesses,imageSources,images,iterations,mail,metrics,onboarding,operations,plugins,scripts,stores,tasks,usage,workflows}.ts`, `ui/src/types/index.ts`
- Modify: the 19 matching modules in `ui/src/lib/api/`
- Test: `ui/src/test/typesOnly.test.ts`

**Interfaces:**
- Produces: `@/types` and `@/types/<domain>` export the same type names that `@/lib/api` exports for that domain today, including `TaskComment`, `TaskEvent` and the hand-written types of spec section 4. No runtime export.

- [ ] **Step 1: Write the failing test** `ui/src/test/typesOnly.test.ts`:
  - `it("emits no JavaScript for any file in src/types")`: for each `*.ts` in `ui/src/types/`, `ts.transpileModule(text, { compilerOptions: { verbatimModuleSyntax: true, module: ts.ModuleKind.ESNext } }).outputText` with `export {};` and blank lines removed is `""`. It also expects at least 20 files.
  - `it("lets each api domain module re-export its types from @/types")`: for each of the 19 domain names, the text of `ui/src/lib/api/<domain>.ts` contains `from "@/types/<domain>"`.
- [ ] **Step 2: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run src/test/typesOnly.test.ts'`. Expected: FAIL (directory missing, fewer than 20 files).
- [ ] **Step 3: Implement.** Move each generated re-export line and each allowed hand-written type (with its comment) from `ui/src/lib/api/<domain>.ts` to `ui/src/types/<domain>.ts`. In the domain module, put `export type * from "@/types/<domain>"`, and import the types that the module itself uses with `import type`. `ApiError`, `Transport`, `ApiClient`, `ApiTarget` and the `XxxApi = ReturnType<…>` types stay in `lib/api`. `ui/src/types/index.ts` is 19 lines `export type * from "./<domain>"`.
- [ ] **Step 4: Run** the test of Step 2, then `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && npx tsc -b --checkers 1'`. Expected: PASS, no type error.
- [ ] **Step 5: Run** `make frontend-check` in the background. Expected: every step `ok`. If Knip reports unused exported types in `src/types/`, add `"src/types/*.ts": ["types"]` to `ignoreIssues` in `ui/knip.json` with the reason "`@/types` re-exports the whole domain for views" in the pull request.
- [ ] **Step 6: Commit** `ui: add the type-only @/types directory`.

### Task 2: Components import types from `@/types`

**Files:**
- Modify: the files under `ui/src/components/` and `ui/src/pages/` whose only use of `@/lib/api` is `import type` (27 files under `ui/src/components/` on 2026-11-05: `rg -l 'import type \{[^}]*\} from "@/lib/api"' ui/src/components`).

**Interfaces:**
- Consumes: `@/types` from Task 1.

- [ ] **Step 1: Change** each `import type { … } from "@/lib/api"` to `import type { … } from "@/types"`. A file that also imports a value from `@/lib/api` keeps the value import and moves only the type import. No other change.
- [ ] **Step 2: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && npx tsc -b --checkers 1 && npm run lint'`. Expected: PASS.
- [ ] **Step 3: Run** `make frontend-check`. Expected: every step `ok`.
- [ ] **Step 4: Commit** `ui: import response types from @/types in components`.

Done when: both commits pass `make frontend-check`, `@/lib/api` still exports every type it exported before (`tsc -b` passes without other changes), and `typesOnly.test.ts` passes.

## Stage 3: Storybook

### Task 3: Storybook setup and the `ui-storybook` check

**Files:**
- Modify: `ui/package.json`, `ui/package-lock.json` (`npm install -D --save-exact storybook@<version> @storybook/react-vite@<version>`), `ui/tsconfig.json` (add the reference `./tsconfig.storybook.json`), `ui/.gitignore` (`storybook-static/`), `ui/.oxlintrc.json` (`ignorePatterns` gets `storybook-static`), `ui/knip.json` (`project` gets `!src/**/*.stories.tsx!`, `!src/mockups/**!`, `!src/fixtures/**!`, `!src/storybook/**!`), `Makefile` (`frontend-check`), `docs/docs/development.mdx` (the step list and the build matrix)
- Create: `ui/.storybook/main.ts`, `ui/.storybook/preview.tsx`, `ui/tsconfig.storybook.json`, `ui/src/storybook/decorators.tsx`, `ui/src/components/ThemeToggle.stories.tsx` (first real story)
- Test: `ui/src/test/storybookDecorators.test.tsx`

`ui/tsconfig.json` keeps its reference to `./tsconfig.design-sync.json` until Task 5 deletes that file, so `tsc -b` checks the old previews until the stories replace them.

**Interfaces:**
- Produces in `ui/src/storybook/decorators.tsx`:
  - `withTheme: Decorator` (reads `context.globals.theme`, `"light" | "dark"`, sets the class `dark` on `document.documentElement`);
  - `withAppProviders: Decorator` (`TooltipProvider` and `MemoryRouter` unless `parameters.tariboy.route` is set);
  - `withQueryClient: Decorator` (a new `createQueryClient()` for each story render, when `parameters.tariboy.query === true`);
  - `withDaemon: Decorator` (`DaemonProvider`, when `parameters.tariboy.daemon` is `true` or `{ daemons, active }`, which it writes to `localStorage` keys `tariboy_daemons` and `tariboy_active_daemon` before the provider mounts);
  - `withRoute: Decorator` (`MemoryRouter initialEntries={[path]}` with `<Routes><Route path={pattern} element={<Story />} /></Routes>`, when `parameters.tariboy.route = { path, pattern }`);
  - `withSurface: (kind: "sidebar" | "panel" | "card" | "row", width?: number) => Decorator`;
  - `type TariboyParameters = { query?: boolean; daemon?: boolean | { daemons: unknown[]; active: string }; route?: { path: string; pattern: string } }`.
- `ui/.storybook/preview.tsx`: `decorators: [withQueryClient, withDaemon, withRoute, withAppProviders, withTheme]` (Storybook applies the last one outermost, so `withTheme` wraps everything and `withRoute` sits inside `withAppProviders`, which then skips its own `MemoryRouter`); `globalTypes.theme` with a toolbar `light`/`dark`, default `light`; imports `../src/index.css`.
- `ui/.storybook/main.ts`: `framework: "@storybook/react-vite"`, `stories: ["../src/**/*.stories.tsx"]`, no addons, `viteFinal` merges `vite.config.ts` aliases (`@` → `src`) and the Tailwind plugin.

- [ ] **Step 1: Write the failing test** `ui/src/test/storybookDecorators.test.tsx`:
  - `it("gives each story render its own query client")`: render a probe story through `withQueryClient` twice with `parameters.tariboy.query = true`; the probe reads `useQueryClient()`; the two clients differ (`not.toBe`).
  - `it("mounts a matched route for a useParams story")`: render a probe that prints `useParams().name` through `withRoute` with `{ path: "/agents/builder", pattern: "/agents/:name" }`; the text is `builder`.
  - `it("puts the dark class on the document for the dark theme")`: `withTheme` with `globals.theme = "dark"`; `document.documentElement.classList.contains("dark")` is `true`, and `false` for `"light"`.
- [ ] **Step 2: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run src/test/storybookDecorators.test.tsx'`. Expected: FAIL, module `@/storybook/decorators` not found.
- [ ] **Step 3: Install** the two packages with the exact version, and implement `decorators.tsx`, `.storybook/main.ts`, `.storybook/preview.tsx`, `tsconfig.storybook.json` (`extends: "./tsconfig.app.json"`, `include: [".storybook"]`, its own `tsBuildInfoFile`), and the scripts `"storybook": "storybook dev -p 6006"` and `"build-storybook": "storybook build"`.
- [ ] **Step 4: Run** the test of Step 2. Expected: PASS.
- [ ] **Step 5: RED story.** Write `ui/src/components/ThemeToggle.stories.tsx` with `meta = { component: ThemeToggle }` and a story `Default` that passes an extra prop `bogus: 1` in `args`. Run `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && npx tsc -b --checkers 1'`. Expected: FAIL with an error in `ThemeToggle.stories.tsx`. Remove the wrong prop, add the stories `Default` and `InTitlebar` of the preview `ui/.design-sync/previews/ThemeToggle.tsx`, and run again. Expected: PASS.
- [ ] **Step 6: Knip and lint.** Run `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && npm run knip && npm run lint'`. Expected: PASS. If the first pass reports `ThemeToggle.stories.tsx` or `src/storybook/decorators.tsx`, the `project` exclusions are missing.
- [ ] **Step 7: Bundle budget.** Before Step 3 (on the base commit) and now, run `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && npm run bundle:budget'`. Expected: the same entry, initial and total numbers.
- [ ] **Step 8: Measure.** `cd ui && flock -o /tmp/tariboy-check.lock /usr/bin/time -v npm run build-storybook -- --output-dir <scratch>/storybook-static`. Record the wall time and `Maximum resident set size` in the pull request. If the peak is over 1.5 GB, set `NODE_OPTIONS=--max-old-space-size=1536` in the step command of Step 9.
- [ ] **Step 9: Add the step** to `frontend-check` in the `Makefile`, after `ui-test`: `run_step "ui-storybook" 'cd ui && npm run build-storybook -- --output-dir "$$TMPDIR/storybook-static" --quiet'`. Add `ui-storybook` to the step list and the build matrix in `docs/docs/development.mdx`, with the measured time and memory.
- [ ] **Step 10: Run** `make frontend-check` in the background. Expected: every step `ok`, `ui-storybook` included.
- [ ] **Step 11: Visual check.** `cd ui && npm run storybook -- --host 127.0.0.1 --ci`, open `http://127.0.0.1:6006`, look at `ThemeToggle` in light and dark.
- [ ] **Step 12: Commit** `ui: add Storybook with the shared decorators and the ui-storybook check`.

Done when: `make frontend-check` passes with `ui-storybook`, the bundle numbers did not change, and a wrong story prop fails `tsc -b`.

## Stage 4: Migration of the previews

### Task 4: Migration guard test

**Files:**
- Create: `ui/src/test/storyMigration.test.ts` (temporary, deleted in Task 5)

- [ ] **Step 1: Write the failing test** `it("has a story with every export of every design-sync preview")`: a map of the 55 previews to story files (spec section 9 table, as a constant in the test). For each preview, the exported names (`/^export const (\w+)/gm`) are a subset of the story file's exported names. A missing story file fails with its path.
- [ ] **Step 2: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run src/test/storyMigration.test.ts'`. Expected: FAIL, 54 missing story files (ThemeToggle exists).
- [ ] **Step 3: Keep the test uncommitted.** Each group of Task 5 runs it and sees fewer failures. It is committed in the group 3 commit of Task 5, when it passes, so no commit on the branch has a failing test.

### Task 5: Stories for the 55 previews, then delete `.design-sync/`

**Files:**
- Create: the 47 story files of spec section 9 (55 previews, `status.tsx` gives one file for three previews, `ThemeToggle` exists).
- Modify (last commit): `ui/.oxlintrc.json`, `ui/knip.json`, `ui/.gitignore`, `ui/package.json` (`fmt`, `fmt:check`), `ui/scripts/complexity-report.mjs`, `ui/src/index.css`, `ui/src/test/lintPolicy.test.ts`, `ui/design_handoff_console_slice/README.md`, `ui/design_handoff_console_slice/PROMPT.md`, `docs/docs/development.mdx`, `docs/docs/architecture/web-ui.mdx` (new section "Designer workflow in Storybook": the style rules and component notes of spec section 7.1)
- Delete (last commit): `ui/.design-sync/`, `ui/tsconfig.design-sync.json`, `ui/src/test/designSyncCatalog.test.ts`, `ui/src/test/designQueryProvider.test.ts`, `ui/src/test/storyMigration.test.ts`

**Interfaces:**
- Consumes: the decorators and `TariboyParameters` of Task 3.

- [ ] **Step 1: Group 1, primitives** (24 previews, order of spec section 9). Each story file: `meta` with `component`, the backdrop as `decorators: [withSurface(…)]`, one named story for each preview export with the same name and the same content. Imports from the component's module, not from `"tariboy-ui"`. `Tooltip` drops its local `TooltipProvider`.
- [ ] **Step 2: Run** the guard test (30 failures left), `tsc -b`, `npm run lint`, `npm run knip` under the slot. Expected: PASS except the guard. Commit the story files only: `ui: move the primitive previews to stories`.
- [ ] **Step 3: Group 2, product components** (24). `SendFilesButton` `TerminalToolbar` uses `withSurface("panel")` without the hex and `--color-zinc-950` values. `Toaster` imports `toast` from `sonner`. Contexts are imported from their source modules. Run the checks of Step 2. Commit `ui: move the component previews to stories`.
- [ ] **Step 4: Group 3, pages and containers** (7). `TasksWorkspace` and `TaskDetail` use `parameters.tariboy = { daemon: true, query: true }` and drop their local `QueryClientProvider`. `AgentWorkspace`, `LibraryImagesPage`, `SettingsPage`, `StoresPage`, `TerminalsPage` use `route`. Run the checks. Expected: the guard test PASSES. Commit the stories and the guard test: `ui: move the page previews to stories`.
- [ ] **Step 5: Visual check** in Storybook (`npm run storybook`): every migrated story in light and dark, compared with the old preview content. Note each difference in the pull request.
- [ ] **Step 6: Delete and replace references.** Apply every row of spec section 10 and 10.1 with stage 4: delete the files and directories, the `.design-sync/**` override (`ui/.oxlintrc.json` lines 102–114), the `./tsconfig.design-sync.json` reference in `ui/tsconfig.json` (line 6), the `@source not` block and the seven `/* @kind … */` comments in `ui/src/index.css`, the `.gitignore` lines 3–7; replace the `fmt` paths, the `lintPolicy.test.ts` override list (`[testOverride]`) and its scanned directories (`["src", "store", ".storybook"]`), the comment of `complexity-report.mjs`, the README line 144 and item 2 above it, `PROMPT.md` line 18, and the `development.mdx` lines. Add the "Designer workflow in Storybook" section to `web-ui.mdx` with spec section 7.1. Delete `ui/ds-bundle/` from your local checkout (not tracked).
- [ ] **Step 7: Run** `rg -i -n 'design-sync|ds-bundle|tariboy-ui"' --hidden -g '!node_modules'` in the worktree. Expected: only the "Keep" rows of spec section 10 (`ui/package.json:2`, `ui/package-lock.json:2`, `ui/package-lock.json:8`, the two `docs/superpowers/` files).
- [ ] **Step 8: Run** `make frontend-check` in the background. Expected: every step `ok`, the `docs` step builds the changed docs.
- [ ] **Step 9: Ask** tariboy-designer, through the PM, to review the "Designer workflow in Storybook" section before acceptance.
- [ ] **Step 10: Commit** `ui: remove the Claude Design catalog`.

Done when: the guard test passed before its deletion, `rg` finds only the "Keep" rows, `make frontend-check` passes, and both themes were checked in Storybook.

## Stage 5: Lint boundaries

### Task 6: oxlint override and the color policy test

**Files:**
- Modify: `ui/.oxlintrc.json` (the override of spec section 3.1, final configuration), `ui/src/test/lintPolicy.test.ts` (expected overrides `[testOverride, viewLayerOverride]`, test name "overrides rules only for test files and the view layer"), `docs/docs/development.mdx` (line 625 paragraph: two override kinds), the stories, mockups and fixtures that violate a rule
- Rename: `ui/src/components/AgentRow.tsx` → `ui/src/components/AgentRow.view.tsx` (and its importers)
- Create: `ui/src/test/viewLintPolicy.test.ts`

**Interfaces:**
- Produces: `viewLayerOverride` in `lintPolicy.test.ts`, equal to the `.oxlintrc.json` entry of spec section 3.1.

- [ ] **Step 1: Write the failing tests** in `ui/src/test/viewLintPolicy.test.ts`:
  - `it("rejects data access in each view-layer glob")`: in a temporary directory, copy `ui/.oxlintrc.json` and write `src/components/x/A.view.tsx`, `src/components/x/A.stories.tsx`, `src/mockups/p/P.stories.tsx`, `src/fixtures/f.ts`, each with `import type { AgentRow } from "@/lib/api"`. Also write into `A.view.tsx` the lines `import { tasksApi } from "@/lib/api/tasks"`, `import { useQuery } from "@tanstack/react-query"`, `export { x } from "@/lib/api"`, `void fetch("/a")`, `void window.fetch("/b")`, and `import type { AgentRow as T } from "@/types"`. Run `oxlint -f unix` from that directory (as `lintPolicy.test.ts` does). Expect `no-restricted-imports` on each `@/lib/api`, `@/lib/api/tasks`, `@tanstack/react-query` line, `no-restricted-globals` on `fetch`, `no-restricted-properties` on `window.fetch`, no error on the `@/types` line, and no error for the same content in `src/components/x/B.tsx`.
  - `it("finds no hex color or Tailwind palette in the view layer")`: the scanner of spec section 3.2 over the real files of the four globs returns `[]`.
  - `it("detects each color form")`: the scanner on a temporary file with `className="bg-red-500"`, `cn("border-[#ff0000]")`, `cva("", { variants: { t: { a: "hover:text-rose-600/50" } } })`, `style={{ color: "#123456", background: "var(--color-zinc-950)" }}` returns five findings, and on `className="bg-primary text-muted-foreground"` returns none.
  - `it("reads 26 palette names from tailwindcss/theme.css")`: the list equals the 26 names of spec section 3.2.
- [ ] **Step 2: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run src/test/viewLintPolicy.test.ts src/test/lintPolicy.test.ts'`. Expected: FAIL in the first test (no override yet). The scanner tests fail if a migrated story still has a palette value.
- [ ] **Step 3: Implement** the override, the scanner `findColorViolations(files: string[], palettes: string[]): string[]` (returns `file:line: match`) in the test file, the `lintPolicy.test.ts` change, and the rename of `AgentRow.tsx`. Fix each violation that `npm run lint` and the scanner report in stories, mockups and fixtures with tokens of spec section 7.1.
- [ ] **Step 4: Run** the tests of Step 2, then `npm run lint` under the slot. Expected: PASS.
- [ ] **Step 5: Run** `make frontend-check`. Expected: every step `ok`.
- [ ] **Step 6: Commit** `ui: forbid data access and raw colors in the view layer`.

Done when: each forbidden form of spec section 3 has a test that fails without the rule, and `make frontend-check` passes.

## Stage 6: Pilot

### Task 7: Split `HomePage` into a view and a container

The pilot component: `ui/src/pages/home/HomePage.tsx` (168 lines). It is on the home page, it calls the data hooks `useTaskActivity`, `useNeedsYou` and `useNavigate`, and it mixes them with markup.

**Files:**
- Create: `ui/src/pages/home/HomePage.view.tsx`, `ui/src/pages/home/HomePage.stories.tsx`
- Modify: `ui/src/pages/home/HomePage.tsx` (container only)
- Test: `ui/src/pages/home/HomePage.test.tsx` stays unchanged and must pass

**Interfaces:**
- Produces in `HomePage.view.tsx`:

```ts
export interface HomeAgentLink {
  key: string;
  name: string;
  /** "12 min · build-01", "idle", already formatted. */
  meta: string;
  href: string;
}
export interface HomeViewProps {
  needsYouCount: number;
  /** The Needs you list. It keeps its own task drawer and data, so the
   *  container renders it. */
  needsYou: ReactNode;
  running: HomeAgentLink[];
  recent: HomeAgentLink[];
  totals: TaskActivityTotals | null;
  todayError: string | null;
  activityHref: string;
  onNewAgent: () => void;
}
export function HomeView(props: HomeViewProps): JSX.Element;
```

  `TaskActivityTotals` comes from `@/types`. The view can render `<Link to={href}>`. It calls no router hook and no data hook.

- [ ] **Step 1: Write the failing story** `HomePage.stories.tsx`: `meta = { component: HomeView }`, stories `Default` (2 running, 8 recent, totals), `Empty` (no running agent, `needsYouCount: 0`), `TodayError` (`todayError: "request timed out"`). Values come from `ui/src/fixtures/agents.ts` (create it with `agentLinks(): HomeAgentLink[]` typed via `@/types`) and literals in the story.
- [ ] **Step 2: Run** `tsc -b` under the slot. Expected: FAIL, `HomePage.view` not found.
- [ ] **Step 3: Implement** `HomeView` from the markup of `HomePage.tsx` (the `Section` helper moves with it). `HomePage.tsx` keeps the hooks, computes `running`, `recent` (with `agentPath(...)` and `elapsed(...)`) and `activityHref`, renders `NeedsYouList` for the `needsYou` slot, and passes `useTaskActivity` results inside the page-host `ApiProvider`.
- [ ] **Step 4: Run** `tsc -b`, `npm run lint`, and `npx vitest run src/pages/home` under the slot. Expected: PASS, `HomePage.test.tsx` unchanged.
- [ ] **Step 5: Commit** `ui: split HomePage into a view and a container`.

### Task 8: Home page mockup and `handoff.md`

**Files:**
- Create: `ui/src/mockups/home/Home.stories.tsx`, `ui/src/mockups/home/handoff.md`, `ui/src/fixtures/activity.ts` (complete `TaskActivityTotals` values)

**Interfaces:**
- Consumes: `HomeView`, `HomeAgentLink` from Task 7; `agentLinks()` from `ui/src/fixtures/agents.ts`.

- [ ] **Step 1: Write** `Home.stories.tsx` with `title: "Mockups/Home"`: the full page from `HomeView`, `AgentRow` (in a sidebar surface) and fixtures, stories `Default` and `Busy` (many running agents). Write `handoff.md` in the format of spec section 6, with the story ids `mockups-home--default` and `mockups-home--busy`.
- [ ] **Step 2: Run** `tsc -b`, `npm run lint`, `npx vitest run src/test/viewLintPolicy.test.ts`, `npm run knip` under the slot. Expected: PASS. A props change in `HomeViewProps` (try: rename `todayError`) makes `tsc -b` fail in both story files; revert it.
- [ ] **Step 3: Run** `make frontend-check`. Expected: every step `ok`.
- [ ] **Step 4: Visual check:** Storybook, both stories in both themes; `make ui-dev` and the real Home page at the printed URL, compared with the mockup.
- [ ] **Step 5: Commit** `ui: add the Home page mockup and its handoff`.

Done when: the view, the container and the mockup compile, `HomePage.test.tsx` passes unchanged, and the visual checks are done.

## Stage 7: Documentation

### Task 9: Contributor and architecture documentation

**Files:**
- Modify: `AGENTS.md` (section "API contract": components import response types from `@/types`, `@/lib/api` re-exports them; a new section "Designer and developer roles" with the ownership table of spec section 1 and the rules of spec section 7.2), `docs/docs/architecture/web-ui.mdx` ("API types": `@/types`; "Designer workflow in Storybook": add the role rules of spec section 7 and the `handoff.md` format of spec section 6), `docs/docs/development.mdx` (a "Storybook" section in place of the design-sync text: commands, decorators, `parameters.tariboy`, the checks, the matrix row of `ui-storybook`, the Knip description), `ui/knip.json` (the `ignoreIssues` reason for `src/components/ui/*.tsx` is the Storybook catalog; remove the `separator.tsx` and `tooltip.tsx` file exceptions if the first Knip pass no longer reports them)

- [ ] **Step 1: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && npm run knip'` without the two file exceptions. Expected: either PASS (remove them) or the two files reported (keep them, with the Storybook reason).
- [ ] **Step 2: Write** the documentation changes. Each statement names a path, a command or a check that exists after stages 2–6.
- [ ] **Step 3: Run** `cd docs && npm run doctor && npm run build` under the slot. Expected: PASS.
- [ ] **Step 4: Run** `make frontend-check`. Expected: every step `ok`.
- [ ] **Step 5: Commit** `docs: describe the Storybook design workflow and the @/types contract`.

Done when: `AGENTS.md`, `web-ui.mdx` and `development.mdx` describe `@/types`, Storybook, the roles and the checks, and `make frontend-check` passes.
