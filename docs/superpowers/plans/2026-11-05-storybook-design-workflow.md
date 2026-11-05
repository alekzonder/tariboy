# Storybook design workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the Claude Design catalog in `ui/.design-sync/` with Storybook stories next to the components, typed against `@/types`, with lint boundaries for the view layer, designer tools to see a story and check a change, one pilot view/container split and one page mockup with `handoff.md`.

**Architecture:** `ui/src/types/` holds type-only re-exports of the generated API types. Storybook (`@storybook/react-vite`) builds the stories from `ui/src` with its own Vite file `ui/vite.storybook.config.ts` (the app's plugins and alias, no `/api` proxy). View files (`*.view.tsx`), stories, mockups and fixtures get an oxlint override and a policy test that keep data access out of them. `story:shot` and `make designer-check` are the designer's tools.

**Tech Stack:** React 19, Vite 8, Tailwind CSS 4.3.0, TypeScript, Vitest, Playwright 1.60, oxlint 1.81.0, Knip 6.40.0, Storybook 10.6.5 (`storybook`, `@storybook/react-vite`).

**Spec:** `docs/superpowers/specs/2026-11-05-storybook-design-workflow-design.md`

## Global Constraints

- Stages run in order 2, 3, 4, 5, 6, 7. Each stage is its own pull request, and the next stage starts after the user accepts the previous one.
- Each stage works in its own branch and worktree. History is linear: rebase, never merge.
- Run every check, oxlint and Vitest only under `flock -o /tmp/tariboy-check.lock`, one at a time. Vitest runs with `TARIBOY_VITEST_MAX_WORKERS=2`.
- Never start a daemon against the live `~/.tariboy`, `~/.tariboyd` or `127.0.0.1:9990`. `make ui-dev` uses its isolated API. Storybook never reaches a daemon (spec 9.1): no stage adds a `/api` proxy to `ui/vite.storybook.config.ts`.
- No commit or pull request carries a co-author trailer or a mention of AI. No version bump.
- Out of scope: redesign of components, token changes, mass split of components into view and container, backend, OpenAPI and `ui/src/lib/api-types.generated.ts` changes, regression screenshots (spec section 11), and the launch configuration of the designer agent (a customer decision).
- `@/lib/api` keeps re-exporting every type it re-exports today.
- Storybook version: `storybook` and `@storybook/react-vite` at the same exact version, the latest 10.x that `npm view @storybook/react-vite version` prints on the day of stage 3 (10.6.5 on 2026-11-05).
- View-layer glob, the same in oxlint and in the policy test: `src/**/*.view.tsx`, `src/**/*.stories.tsx`, `src/mockups/**`, `src/fixtures/**`.
- Supported widths: 360, 390, 820, 1024, 1180 px (`ui/playwright.responsive.config.ts`).
- Before each commit: `git diff --check` and a read of the whole diff.
- Check of a stage: `make frontend-check` (UI and docs only). `make check` when a stage touches the `Makefile` or files outside `ui/` and `docs/` (stages 3 and 5).

## Review Focus

- Safety: a story must never reach the live daemon. Task 3 pins this with the config test, the `withDaemon` test and the safety test, and with a manual network check.
- A story that passes a wrong prop must fail `tsc -b`. A story under `src/` outside `tsconfig.app.json` would pass silently. Task 3 pins this with its RED story.
- A component that reads `useParams()` (AgentWorkspace, ImageLayout, LibraryImagesPage) renders empty under a plain `MemoryRouter`. Task 6 checks that each such story mounts a matched route.
- The dark theme: a story that looks right only in light mode. Task 6 and Task 11 check both themes with `story:shot`.
- The first Knip pass reports stories, fixtures and decorators as unused files when `project` does not exclude them. Task 3 pins this with `npm run knip` after the first story.
- The oxlint override silently stops applying after a rename of the glob or an oxlint update. Task 7 runs oxlint on violation files as a test, not only a config compare.

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

### Task 3: Storybook setup, daemon isolation and the `ui-storybook` check

**Files:**
- Modify: `ui/package.json`, `ui/package-lock.json` (`npm install -D --save-exact storybook@<version> @storybook/react-vite@<version>`), `ui/tsconfig.json` (add the reference `./tsconfig.storybook.json`), `ui/tsconfig.node.json` (`include` gets `vite.storybook.config.ts`, next to `vite.desktop.config.ts` and `vite.store.config.ts`), `ui/.gitignore` (`storybook-static/`), `ui/.oxlintrc.json` (`ignorePatterns` gets `storybook-static`), `ui/knip.json` (`project` gets `!src/**/*.stories.tsx!`, `!src/mockups/**!`, `!src/fixtures/**!`, `!src/storybook/**!`), `Makefile` (`frontend-check`), `docs/docs/development.mdx` (the step list and the build matrix)
- Create: `ui/vite.storybook.config.ts`, `ui/.storybook/main.ts`, `ui/.storybook/preview.tsx`, `ui/tsconfig.storybook.json`, `ui/src/storybook/decorators.tsx`, `ui/src/components/ThemeToggle.stories.tsx` (first real story)
- Test: `ui/src/test/storybookConfig.test.ts`, `ui/src/test/storybookDecorators.test.tsx`, `ui/src/test/storybookSafety.test.ts`

`ui/tsconfig.json` keeps its reference to `./tsconfig.design-sync.json` until Task 6 deletes that file, so `tsc -b` checks the old previews until the stories replace them.

**Interfaces:**
- `ui/vite.storybook.config.ts`: `defineConfig({ plugins: [react(), tailwindcss()], resolve: { alias: { "@": path.resolve(__dirname, "src") } }, server: { fs: { allow: fsAllow } } })`. No `server.proxy`, no `test` block. Spec 9.1 gives the reason for a separate file.
- `ui/.storybook/main.ts`: `framework: { name: "@storybook/react-vite", options: { builder: { viteConfigPath: "vite.storybook.config.ts" } } }`, `stories: ["../src/**/*.stories.tsx"]`, no addons, no `viteFinal`.
- Produces in `ui/src/storybook/decorators.tsx`:
  - `withTheme: Decorator` (reads `context.globals.theme`, `"light" | "dark"`, sets the class `dark` on `document.documentElement`);
  - `withAppProviders: Decorator` (`TooltipProvider` and `MemoryRouter` unless `parameters.tariboy.route` is set);
  - `withQueryClient: Decorator` (a new `createQueryClient()` for each story render, when `parameters.tariboy.query === true`);
  - `STORYBOOK_DAEMON_URL = "http://storybook.invalid"`;
  - `withDaemon: Decorator` (when `parameters.tariboy.daemon === true`: saves `tariboy_daemons` and `tariboy_active_daemon`, writes a registry with one host `{ id: "storybook", label: "Storybook (no daemon)", baseURL: STORYBOOK_DAEMON_URL }` and makes it active before `DaemonProvider` mounts, and restores both keys, or removes a key that did not exist, in the cleanup);
  - `withRoute: Decorator` (`MemoryRouter initialEntries={[path]}` with `<Routes><Route path={pattern} element={<Story />} /></Routes>`, when `parameters.tariboy.route = { path, pattern }`);
  - `withSurface: (kind: "sidebar" | "panel" | "card" | "row", width?: number) => Decorator`;
  - `withWidth: (width: number) => Decorator` (a frame of that width and `100vh`);
  - `type TariboyParameters = { query?: boolean; daemon?: boolean; route?: { path: string; pattern: string } }`.
- `ui/.storybook/preview.tsx`: `decorators: [withQueryClient, withDaemon, withRoute, withAppProviders, withTheme]` (Storybook applies the last one outermost, so `withTheme` wraps everything and `withRoute` sits inside `withAppProviders`, which then skips its own `MemoryRouter`); `globalTypes.theme` with a toolbar `light`/`dark`, default `light`; imports `../src/index.css`.

- [ ] **Step 1: Write the failing tests.**
  - `ui/src/test/storybookConfig.test.ts`:
    - `it("points Storybook at the Vite file without a proxy")`: import `../../.storybook/main`; `framework.options.builder.viteConfigPath` is `"vite.storybook.config.ts"`, and `viteFinal` is not defined.
    - `it("has no /api proxy and no live listener in the Storybook Vite config")`: `loadConfigFromFile({ command: "serve", mode: "development" }, "vite.storybook.config.ts")` of `vite`; `config.server?.proxy` is `undefined`, and `JSON.stringify(config)` contains neither `"/api"` nor `127.0.0.1:9990`.
  - `ui/src/test/storybookDecorators.test.tsx`:
    - `it("gives each story render its own query client")`: render a probe story through `withQueryClient` twice with `parameters.tariboy.query = true`; the probe reads `useQueryClient()`; the two clients differ (`not.toBe`).
    - `it("mounts a matched route for a useParams story")`: render a probe that prints `useParams().name` through `withRoute` with `{ path: "/agents/builder", pattern: "/agents/:name" }`; the text is `builder`.
    - `it("puts the dark class on the document for the dark theme")`: `withTheme` with `globals.theme = "dark"`; `document.documentElement.classList.contains("dark")` is `true`, and `false` for `"light"`.
    - `it("points the daemon registry at an .invalid host")`: render a probe that reads the active daemon of `DaemonProvider` through `withDaemon`; its `baseURL` is `STORYBOOK_DAEMON_URL` and `new URL(baseURL).hostname` ends in `.invalid`.
    - `it("restores both registry keys after each story")`: set `tariboy_daemons` to a value and remove `tariboy_active_daemon`; render and unmount a story through `withDaemon`; `tariboy_daemons` has the same value, and `tariboy_active_daemon` is missing again.
  - `ui/src/test/storybookSafety.test.ts`: `it("keeps CustomerQuestionNotifications out of stories and mockups")`: no file of `src/**/*.stories.tsx` and `src/mockups/**` has an import from a path that ends in `/CustomerQuestionNotifications` (case-sensitive). A self-test case: the scanner finds it in a temporary file, and does not report `@/components/customerQuestionNotificationsContext`.
- [ ] **Step 2: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run src/test/storybookConfig.test.ts src/test/storybookDecorators.test.tsx src/test/storybookSafety.test.ts'`. Expected: FAIL: `.storybook/main` and `@/storybook/decorators` not found. The safety test passes on the real files (no story yet), and its self-test fails until the scanner exists.
- [ ] **Step 3: Install** the two packages with the exact version, and implement `vite.storybook.config.ts`, `decorators.tsx`, `.storybook/main.ts`, `.storybook/preview.tsx`, `tsconfig.storybook.json` (`extends: "./tsconfig.app.json"`, `include: [".storybook"]`, its own `tsBuildInfoFile`), the scanner of the safety test, and the scripts `"storybook": "storybook dev -p 6006 --host 127.0.0.1"` and `"build-storybook": "storybook build"`.
- [ ] **Step 4: Run** the tests of Step 2. Expected: PASS.
- [ ] **Step 5: RED story.** Write `ui/src/components/ThemeToggle.stories.tsx` with `meta = { component: ThemeToggle }` and a story `Default` that passes an extra prop `bogus: 1` in `args`. Run `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && npx tsc -b --checkers 1'`. Expected: FAIL with an error in `ThemeToggle.stories.tsx`. Remove the wrong prop, add the stories `Default` and `InTitlebar` of the preview `ui/.design-sync/previews/ThemeToggle.tsx`, and run again. Expected: PASS.
- [ ] **Step 6: Knip and lint.** Run `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && npm run knip && npm run lint'`. Expected: PASS. If the first pass reports `ThemeToggle.stories.tsx` or `src/storybook/decorators.tsx`, the `project` exclusions are missing.
- [ ] **Step 7: Bundle budget.** Before Step 3 (on the base commit) and now, run `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && npm run bundle:budget'`. Expected: the same entry, initial and total numbers.
- [ ] **Step 8: Measure.** `cd ui && flock -o /tmp/tariboy-check.lock /usr/bin/time -v npm run build-storybook -- --output-dir <scratch>/storybook-static`. Record the wall time and `Maximum resident set size` in the pull request. If the peak is over 1.5 GB, set `NODE_OPTIONS=--max-old-space-size=1536` in the step command of Step 9.
- [ ] **Step 9: Add the step** to `frontend-check` in the `Makefile`, after `ui-test`: `run_step "ui-storybook" 'cd ui && npm run build-storybook -- --output-dir "$$TMPDIR/storybook-static" --quiet'`. Add `ui-storybook` to the step list and the build matrix in `docs/docs/development.mdx`, with the measured time and memory.
- [ ] **Step 10: Run** `make check` in the background (the `Makefile` changed). Expected: every step `ok`, `ui-storybook` included.
- [ ] **Step 11: Visual check.** `cd ui && npm run storybook -- --ci`, open `http://127.0.0.1:6006`, look at `ThemeToggle` in light and dark.
- [ ] **Step 12: Manual network check.** With `npm run storybook` running, add a temporary page story with `parameters.tariboy = { daemon: true, route: … }` (for example `StoresPage`), open it in a Playwright script with `page.on("request")`, and list the requests. Expected: no request to `127.0.0.1:9990` and no request to a relative `/api` (the origin `127.0.0.1:6006` with a path that starts with `/api`); requests to `storybook.invalid` fail. Delete the temporary story. Record the request list in the pull request.
- [ ] **Step 13: Commit** `ui: add Storybook with daemon isolation and the ui-storybook check`.

### Task 4: `story:shot`, a snapshot of one story for the designer

**Files:**
- Create: `ui/scripts/story-shot.mjs`
- Modify: `ui/package.json` (`"story:shot": "node scripts/story-shot.mjs"`), `ui/.gitignore` (`.story-shots/`), `ui/.oxlintrc.json` (`ignorePatterns` gets `.story-shots`), `docs/docs/development.mdx` (the command in the Storybook text)
- Test: `ui/src/test/storyShot.test.ts`

**Interfaces:**
- Command: `cd ui && npm run story:shot -- <story-id> <light|dark> <width> [--rebuild]`.
- Exports of `story-shot.mjs` (the `main` runs only when the file is the entry):
  - `parseArgs(argv: string[]): { id: string; theme: "light" | "dark"; width: number; rebuild: boolean }`: throws a usage error for a missing id, a theme other than `light` or `dark`, or a width that is not a positive integer;
  - `storyUrl(origin: string, id: string, theme: string): string`: `${origin}/iframe.html?id=<encoded id>&viewMode=story&globals=theme:<theme>`;
  - `shotPath(uiDir: string, id: string, theme: string, width: number): string`: `<uiDir>/.story-shots/<id>.<theme>.<width>.png`;
  - `serveStatic(dir: string): Promise<http.Server>`: a `node:http` static server on `127.0.0.1`, port `0`;
  - `slotCommand(env, argv): string[] | null`: `["flock", "-w", "3600", <lock>, process.execPath, <script>, ...argv]` with `<lock>` = `env.TARIBOY_CHECK_LOCK ?? "/tmp/tariboy-check.lock"`, or `null` when `env.TARIBOY_CHECK_LOCK_HELD` is set.
- Flow of `main`: take the host check slot (run itself again through `slotCommand` with `TARIBOY_CHECK_LOCK_HELD=1`, as `CHECK_SLOT` of the `Makefile` does); run `npm run build-storybook -- --output-dir storybook-static --quiet` if `storybook-static/index.json` is missing or `--rebuild` is given; exit with an error if `index.json` has no entry with the id; start `serveStatic`; open the URL in Playwright `chromium` from `ui/node_modules` with the viewport `width` × 800 and `fullPage: true`; wait for `document.fonts.ready`; write the PNG; print its path; close the browser and the server.

- [ ] **Step 1: Write the failing test** `ui/src/test/storyShot.test.ts`:
  - `it("parses the id, the theme and the width")`, and `it("rejects a theme other than light or dark")`, `it("rejects a width that is not a positive integer")`;
  - `it("builds the iframe URL with the theme global")`: `storyUrl("http://127.0.0.1:41234", "mockups-home--width360", "dark")` equals `http://127.0.0.1:41234/iframe.html?id=mockups-home--width360&viewMode=story&globals=theme:dark`;
  - `it("writes the PNG under .story-shots")`: `shotPath("/x/ui", "a--b", "light", 360)` is `/x/ui/.story-shots/a--b.light.360.png`;
  - `it("serves only on loopback")`: `serveStatic(tmpDir)` returns a server whose `address()` is `{ address: "127.0.0.1", port: > 0 }`; a GET of a file returns its content; close it;
  - `it("takes the host check slot unless it already holds it")`: `slotCommand({}, argv)` starts with `flock` and names `/tmp/tariboy-check.lock`; `slotCommand({ TARIBOY_CHECK_LOCK_HELD: "1" }, argv)` is `null`.
- [ ] **Step 2: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run src/test/storyShot.test.ts'`. Expected: FAIL, `scripts/story-shot.mjs` not found.
- [ ] **Step 3: Implement** `story-shot.mjs` and the `package.json`, `.gitignore` and `.oxlintrc.json` changes.
- [ ] **Step 4: Run** the test of Step 2. Expected: PASS.
- [ ] **Step 5: Manual run.** `cd ui && npm run story:shot -- components-themetoggle--default light 360` and the same with `dark`. Expected: two PNG paths printed; open both and see the toggle in each theme. Run again with `--rebuild` and see the build run.
- [ ] **Step 6: Run** `make frontend-check` in the background. Expected: every step `ok`.
- [ ] **Step 7: Commit** `ui: add story:shot to see one story in a theme and a width`.

Done when: `make check` passes with `ui-storybook`, the bundle numbers did not change, a wrong story prop fails `tsc -b`, the network check finds no request to a daemon, and `story:shot` writes PNGs in both themes.

## Stage 4: Migration of the previews

### Task 5: Fixtures with factories, time and edge sets

**Files:**
- Create: `ui/src/fixtures/time.ts`, `ui/src/fixtures/agents.ts`, `ui/src/fixtures/tasks.ts`, `ui/src/fixtures/images.ts`, `ui/src/fixtures/operations.ts`, `ui/src/fixtures/metrics.ts` (one file for each domain that the stories of Task 6 use; add a domain file when a story needs it)
- Modify: `ui/src/test/fixtures/apiFixtures.ts` (re-export the moved factories, so the tests keep their imports)
- Test: `ui/src/test/storyFixtures.test.ts`

**Interfaces:**
- `ui/src/fixtures/time.ts`: `export const NOW = "2026-11-05T10:00:00Z"`, `minutesAgo(n: number): string`, `hoursAgo(n: number): string`, both relative to `NOW`.
- Each domain file: the factories `<entity>(over: Partial<T> = {}): T` with complete defaults, typed with `@/types` (`agentRow`, `agentView`, `agentBudget` in `agents.ts`; `taskItem` in `tasks.ts`; `imageSummary` in `images.ts`; `operation`, `operationStarted` in `operations.ts`; `metricsSlice`, `metricsReport`, `taskMetrics` in `metrics.ts`), and edge sets `<entities>.empty`, `.one`, `.many` (50 items) and `.longText`. Example: `agentRows.many`.
- Dates in defaults come from `NOW`. Names come from the product vocabulary of spec section 5.

- [ ] **Step 1: Write the failing test** `ui/src/test/storyFixtures.test.ts`:
  - `it("keeps the factories of apiFixtures")`: for each moved name, `apiFixtures.<name>` is the same function as `@/fixtures/<domain>.<name>` (`toBe`).
  - `it("applies the overrides over complete defaults")`: `agentRow({ name: "builder", state: "running" })` has `name` `builder`, `state` `running`, and every other key of `agentRow()`.
  - `it("has empty, one, many and longText sets")`: for each domain file, `.empty` has 0 items, `.one` 1, `.many` 50, and `.longText` has a name of at least 60 characters.
  - `it("takes every date from NOW")`: each ISO date string in the defaults of the factories is not later than `NOW`, and `minutesAgo(5)` is `2026-11-05T09:55:00.000Z`.
- [ ] **Step 2: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run src/test/storyFixtures.test.ts'`. Expected: FAIL, `@/fixtures/time` not found.
- [ ] **Step 3: Implement.** Move the factories from `apiFixtures.ts` to the domain files, change their imports to `@/types`, replace their dates with `NOW`-relative values, add the edge sets, and re-export the factories from `apiFixtures.ts`.
- [ ] **Step 4: Run** the test of Step 2 and the whole UI test suite under the slot (`cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run`). Expected: PASS. A test that compared a fixed date of an old default gets the `NOW` value; note each such change in the pull request.
- [ ] **Step 5: Commit** `ui: add story fixtures with factories, a fixed NOW and edge sets`.

### Task 6: Stories for 54 of the 55 previews, then delete `.design-sync/`

**Files:**
- Create: `ui/src/test/storyMigration.test.ts` (temporary, deleted in Step 8), the 51 story files of spec section 9.2 (54 previews; `status.tsx` gives one file for three previews; `ThemeToggle` exists; `CustomerQuestionNotifications` is not migrated)
- Modify (last commit): `ui/.oxlintrc.json`, `ui/knip.json`, `ui/.gitignore`, `ui/package.json` (`fmt`, `fmt:check`), `ui/scripts/complexity-report.mjs`, `ui/src/index.css`, `ui/src/test/lintPolicy.test.ts`, `ui/design_handoff_console_slice/README.md`, `ui/design_handoff_console_slice/PROMPT.md`, `docs/docs/development.mdx`, `docs/docs/architecture/web-ui.mdx` (new section "Designer workflow in Storybook": the style rules, the work rules and the component notes of spec section 7.1)
- Delete (last commit): `ui/.design-sync/`, `ui/tsconfig.design-sync.json`, `ui/src/test/designSyncCatalog.test.ts`, `ui/src/test/designQueryProvider.test.ts`, `ui/src/test/storyMigration.test.ts`

**Interfaces:**
- Consumes: the decorators and `TariboyParameters` of Task 3, the fixtures of Task 5.

- [ ] **Step 1: Write the failing guard test** `it("has a story with every export of every design-sync preview")`: a map of the 54 previews to story files (spec section 9.2 table, as a constant in the test; `CustomerQuestionNotifications` is not in the map). For each preview, the exported names (`/^export const (\w+)/gm`) are a subset of the story file's exported names. A missing story file fails with its path. Run `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run src/test/storyMigration.test.ts'`. Expected: FAIL, 51 missing story files. Keep the test uncommitted until Step 4, so no commit on the branch has a failing test.
- [ ] **Step 2: Group 1, primitives** (24 previews, 22 files, order of spec section 9.2). Each story file: `meta` with `component`, the backdrop as `decorators: [withSurface(…)]`, one named story for each preview export with the same name and the same content. Imports from the component's module, not from `"tariboy-ui"`. Values from `@/fixtures` where a fixture exists. `Tooltip` drops its local `TooltipProvider`. Run the guard test (29 missing files left), `tsc -b`, `npm run lint`, `npm run knip` under the slot. Expected: PASS except the guard. Commit the story files only: `ui: move the primitive previews to stories`.
- [ ] **Step 3: Group 2, product components** (23 previews, 22 files). `SendFilesButton` `TerminalToolbar` renders the toolbar under a `TuiScreen` with a fake `controller` (as in the `TuiScreen` stories), without the hex and `--color-zinc-950` values. `Toaster` imports `toast` from `sonner`. Contexts are imported from their source modules. Run the checks of Step 2 (7 missing files left). Commit `ui: move the component previews to stories`.
- [ ] **Step 4: Group 3, pages and containers** (7). Each page story uses `parameters: { layout: "fullscreen", tariboy: { daemon: true, … } }` and `withWidth(1024)`. `TasksWorkspace` and `TaskDetail` add `query: true` and drop their local `QueryClientProvider`. `AgentWorkspace`, `LibraryImagesPage`, `SettingsPage`, `StoresPage`, `TerminalsPage` use `route`. No story imports `CustomerQuestionNotifications`. Run the checks and `storybookSafety.test.ts`. Expected: the guard test PASSES. Commit the stories and the guard test: `ui: move the page previews to stories`.
- [ ] **Step 5: Visual check** with `story:shot`: every migrated story in `light` and `dark` at 1024 px, and the page stories also at 360 px, compared with the old preview content. Note each difference in the pull request.
- [ ] **Step 6: Delete and replace references.** Apply every row of spec section 10 and 10.1 with stage 4: delete the files and directories, the `.design-sync/**` override (`ui/.oxlintrc.json` lines 102–114), the `./tsconfig.design-sync.json` reference in `ui/tsconfig.json` (line 6), the `@source not` block and the seven `/* @kind … */` comments in `ui/src/index.css`, the `.gitignore` lines 3–7; replace the `fmt` paths, the `lintPolicy.test.ts` override list (`[testOverride]`) and its scanned directories (`["src", "store", ".storybook"]`), the comment of `complexity-report.mjs`, the README line 144 and item 2 above it, `PROMPT.md` line 18, and the `development.mdx` lines. Add the "Designer workflow in Storybook" section to `web-ui.mdx` with all of spec section 7.1: the style rules with the `Card` example, the work rules (local view parts, `story:shot`, the handoff path; `make designer-check` is added in Task 8) and the component notes. Delete `ui/ds-bundle/` from your local checkout (not tracked).
- [ ] **Step 7: Run** `rg -i -n 'design-sync|ds-bundle|tariboy-ui"' --hidden -g '!node_modules'` in the worktree. Expected: only the "Keep" rows of spec section 10 (`ui/package.json:2`, `ui/package-lock.json:2`, `ui/package-lock.json:8`, the two `docs/superpowers/` files).
- [ ] **Step 8: Run** `make frontend-check` in the background. Expected: every step `ok`, the `docs` step builds the changed docs.
- [ ] **Step 9: Ask** tariboy-designer, through the PM, to review the "Designer workflow in Storybook" section before acceptance.
- [ ] **Step 10: Commit** `ui: remove the Claude Design catalog`.

Done when: the fixture test passes, the guard test passed before its deletion, `rg` finds only the "Keep" rows, `make frontend-check` passes, and both themes were checked with `story:shot`.

## Stage 5: Lint boundaries and the designer check

### Task 7: oxlint override and the color policy test

**Files:**
- Modify: `ui/.oxlintrc.json` (the override of spec section 3.1, final configuration), `ui/src/test/lintPolicy.test.ts` (expected overrides `[testOverride, viewLayerOverride]`, test name "overrides rules only for test files and the view layer"), `docs/docs/development.mdx` (line 625 paragraph: two override kinds), `docs/docs/architecture/web-ui.mdx` (view-layer rules name their checks), the stories, mockups and fixtures that violate a rule
- Rename: `ui/src/components/AgentRow.tsx` → `ui/src/components/AgentRow.view.tsx` (and its importers)
- Create: `ui/src/test/viewLintPolicy.test.ts`

**Interfaces:**
- Produces: `viewLayerOverride` in `lintPolicy.test.ts`, equal to the `.oxlintrc.json` entry of spec section 3.1.

- [ ] **Step 1: Write the failing tests** in `ui/src/test/viewLintPolicy.test.ts`:
  - `it("rejects data access in each view-layer glob")`: in a temporary directory, copy `ui/.oxlintrc.json` and write `src/components/x/A.view.tsx`, `src/components/x/A.stories.tsx`, `src/mockups/p/P.stories.tsx`, `src/fixtures/f.ts`, each with `import type { AgentRow } from "@/lib/api"`. Also write into `A.view.tsx` the lines `import { tasksApi } from "@/lib/api/tasks"`, `import { useQuery } from "@tanstack/react-query"`, `export { x } from "@/lib/api"`, `void fetch("/a")`, `void window.fetch("/b")`, and `import type { AgentRow as T } from "@/types"`. Run `oxlint -f unix` from that directory (as `lintPolicy.test.ts` does). Expect `no-restricted-imports` on each `@/lib/api`, `@/lib/api/tasks`, `@tanstack/react-query` line, `no-restricted-globals` on `fetch`, `no-restricted-properties` on `window.fetch`, no error on the `@/types` line, and no error for the same content in `src/components/x/B.tsx`.
  - `it("finds no hex color or Tailwind palette in the view layer")`: the scanner of spec section 3.2 over the real `.ts` and `.tsx` files of the four globs returns `[]`.
  - `it("detects each color form")`: the scanner on a temporary file with `className="bg-red-500"`, `cn("border-[#ff0000]")`, `cva("", { variants: { t: { a: "hover:text-rose-600/50" } } })`, `style={{ color: "#123456", background: "var(--color-zinc-950)" }}` returns five findings, and on `className="bg-primary text-muted-foreground"` returns none. A `handoff.md` with `#123456` in a temporary `src/mockups/p/` is not scanned.
  - `it("reads 26 palette names from tailwindcss/theme.css")`: the list equals the 26 names of spec section 3.2.
- [ ] **Step 2: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run src/test/viewLintPolicy.test.ts src/test/lintPolicy.test.ts'`. Expected: FAIL in the first test (no override yet). The scanner tests fail if a migrated story still has a palette value.
- [ ] **Step 3: Implement** the override, the scanner `findColorViolations(files: string[], palettes: string[]): string[]` (returns `file:line: match`) in the test file, the `lintPolicy.test.ts` change, and the rename of `AgentRow.tsx`. Fix each violation that `npm run lint` and the scanner report in stories, mockups and fixtures with tokens of spec section 7.1.
- [ ] **Step 4: Run** the tests of Step 2, then `npm run lint` under the slot. Expected: PASS.
- [ ] **Step 5: Run** `make frontend-check`. Expected: every step `ok`.
- [ ] **Step 6: Commit** `ui: forbid data access and raw colors in the view layer`.

### Task 8: `make designer-check`

**Files:**
- Modify: `Makefile` (target `designer-check`, `.PHONY`, and the new contract test in `smoke-contract-test`), `docs/docs/development.mdx` (the target in the Storybook text and the matrix), `docs/docs/architecture/web-ui.mdx` (the hand-off rule of spec section 7.1 names the target)
- Create: `scripts/designer-check-contract-test.sh`

**Interfaces:**
- `make designer-check` takes `CHECK_SLOT` and runs, in order and to the end (like `frontend-check`, it does not stop at the first failure, and it fails if a step failed):
  1. `cd ui && npx tsc -b --checkers 1`;
  2. oxlint on the UI files changed against `origin/main`: `git diff --name-only --diff-filter=d "$(git merge-base origin/main HEAD)" -- 'ui/*.ts' 'ui/*.tsx'` (this includes uncommitted changes), with the `ui/` prefix removed, then `cd ui && npx oxlint <files>`; with no file the step prints `no changed UI files` and passes;
  3. `cd ui && TARIBOY_VITEST_MAX_WORKERS=2 npx vitest run src/test/viewLintPolicy.test.ts`;
  4. `cd ui && npm run build-storybook -- --output-dir "$$TMPDIR/storybook-static" --quiet`.

- [ ] **Step 1: Write the failing test** `scripts/designer-check-contract-test.sh`, in the form of `scripts/check-output-contract-test.sh`: `make --no-print-directory -n -C "$repo_root" designer-check` prints a recipe that contains `tariboy-check.lock`, `tsc -b`, `merge-base origin/main`, `oxlint`, `viewLintPolicy.test.ts`, `TARIBOY_VITEST_MAX_WORKERS=2` and `build-storybook`, in this order. Add it to `smoke-contract-test`.
- [ ] **Step 2: Run** `bash scripts/designer-check-contract-test.sh`. Expected: FAIL, `No rule to make target 'designer-check'`.
- [ ] **Step 3: Implement** the target with the step runner of `frontend-check`.
- [ ] **Step 4: Run** the test of Step 2. Expected: PASS.
- [ ] **Step 5: Manual run.** In the stage branch, change one story, run `make designer-check`, and see four `ok` steps and the changed file in the oxlint step. Add `className="bg-red-500"` to the story and run again: the `viewLintPolicy` step fails. Revert.
- [ ] **Step 6: Run** `make check` in the background (the `Makefile` changed). Expected: every step `ok`.
- [ ] **Step 7: Commit** `make: add designer-check for designer hand-offs`.

Done when: each forbidden form of spec section 3 has a test that fails without the rule, `designer-check` has a contract test and a manual run, and `make check` passes.

## Stage 6: Pilot

### Task 9: Split `HomePage` into a view and a container

The pilot component: `ui/src/pages/home/HomePage.tsx` (168 lines). It is on the home page, it calls the data hooks `useTaskActivity`, `useNeedsYou` and `useNavigate`, and it mixes them with markup.

**Files:**
- Create: `ui/src/pages/home/HomePage.view.tsx`, `ui/src/pages/home/HomePage.stories.tsx`
- Modify: `ui/src/pages/home/HomePage.tsx` (container only), `ui/src/fixtures/agents.ts` (`agentLinks`)
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

- [ ] **Step 1: Write the failing story** `HomePage.stories.tsx`: `meta = { component: HomeView }`, stories `Default` (2 running, 8 recent, totals), `Empty` (no running agent, `needsYouCount: 0`), `TodayError` (`todayError: "request timed out"`). Values come from `agentLink(over)` and the edge sets in `ui/src/fixtures/agents.ts` (add the factory `agentLink(over: Partial<HomeAgentLink> = {}): HomeAgentLink` and the sets `agentLinks.empty`, `.one`, `.many`, `.longText`).
- [ ] **Step 2: Run** `tsc -b` under the slot. Expected: FAIL, `HomePage.view` not found.
- [ ] **Step 3: Implement** `HomeView` from the markup of `HomePage.tsx` (the `Section` helper moves with it). `HomePage.tsx` keeps the hooks, computes `running`, `recent` (with `agentPath(...)` and `elapsed(...)`) and `activityHref`, renders `NeedsYouList` for the `needsYou` slot, and passes `useTaskActivity` results inside the page-host `ApiProvider`.
- [ ] **Step 4: Run** `tsc -b`, `npm run lint`, and `npx vitest run src/pages/home` under the slot. Expected: PASS, `HomePage.test.tsx` unchanged.
- [ ] **Step 5: Commit** `ui: split HomePage into a view and a container`.

### Task 10: Home page mockup and `handoff.md`

**Files:**
- Create: `ui/src/mockups/home/Home.stories.tsx`, `ui/src/mockups/home/handoff.md`, `ui/src/fixtures/activity.ts` (factory `taskActivityTotals(over)` with complete defaults, dates from `NOW`)

**Interfaces:**
- Consumes: `HomeView`, `HomeAgentLink` from Task 9; `agentLink`, `agentLinks` from `ui/src/fixtures/agents.ts`; `withWidth`; `story:shot`; `make designer-check`.

- [ ] **Step 1: Write** `Home.stories.tsx` with `title: "Mockups/Home"` and `parameters: { layout: "fullscreen" }`: the full page from `HomeView`, `AgentRow` (in a sidebar surface) and fixtures. Stories: one for each supported width (`Width360`, `Width390`, `Width820`, `Width1024`, `Width1180`, each with `withWidth`) and `Busy` (`agentLinks.many`) at 1024 px. Write `handoff.md` in the format of spec section 6, all twelve sections, with the key of the stage 6 design task and the story ids `mockups-home--width360` … `mockups-home--busy`.
- [ ] **Step 2: Run** `make designer-check`. Expected: four `ok` steps. A props change in `HomeViewProps` (try: rename `todayError`) makes the `tsc -b` step fail in both story files; revert it.
- [ ] **Step 3: Run** `make frontend-check`. Expected: every step `ok`.
- [ ] **Step 4: Visual check:** `story:shot` of each story in both themes; `make ui-dev` and the real Home page at the printed URL, compared with the mockup at 360 px and 1024 px.
- [ ] **Step 5: Commit** `ui: add the Home page mockup and its handoff`.

Done when: the view, the container and the mockup compile, `HomePage.test.tsx` passes unchanged, `make designer-check` passes, and the visual checks are done.

## Stage 7: Documentation

### Task 11: Contributor and architecture documentation

**Files:**
- Modify: `AGENTS.md` (section "API contract": components import response types from `@/types`, `@/lib/api` re-exports them; a new section "Designer and developer roles" with the ownership table of spec section 1 and the rules of spec section 7.2; the routing table gets the row "Designer stories, mockups, fixtures, handoff → `docs/docs/architecture/web-ui.mdx` (Designer workflow in Storybook)"), `docs/docs/architecture/web-ui.mdx` ("API types": `@/types`; "Designer workflow in Storybook": add the role rules of spec section 7, the hand-off path with `make designer-check`, the `handoff.md` format of spec section 6, the fixture rules of spec section 5 and the safety rule of spec 9.1), `docs/docs/development.mdx` (a "Storybook" section in place of the design-sync text: commands, `story:shot`, `make designer-check`, decorators, `parameters.tariboy`, the daemon isolation, the checks, the matrix row of `ui-storybook`, the Knip description), `ui/knip.json` (the `ignoreIssues` reason for `src/components/ui/*.tsx` is the Storybook catalog; remove the `separator.tsx` and `tooltip.tsx` file exceptions if the first Knip pass no longer reports them)

- [ ] **Step 1: Run** `flock -o /tmp/tariboy-check.lock sh -c 'cd ui && npm run knip'` without the two file exceptions. Expected: either PASS (remove them) or the two files reported (keep them, with the Storybook reason).
- [ ] **Step 2: Write** the documentation changes. Each statement names a path, a command or a check that exists after stages 2–6.
- [ ] **Step 3: Run** `cd docs && npm run doctor && npm run build` under the slot. Expected: PASS.
- [ ] **Step 4: Run** `make frontend-check`. Expected: every step `ok`.
- [ ] **Step 5: Commit** `docs: describe the Storybook design workflow and the @/types contract`.

Done when: `AGENTS.md` (with the routing row), `web-ui.mdx` and `development.mdx` describe `@/types`, Storybook, the roles, the designer tools and the checks, and `make frontend-check` passes.
