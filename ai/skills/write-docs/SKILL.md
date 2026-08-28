---
name: write-docs
description: Use when creating or revising English project documentation, including Blume product pages, contributor guides, architecture and workflow documentation, or repository design records.
---

# Write Project Documentation

## Core principles

Create documentation that helps a defined reader achieve a defined goal.

- Write the final documentation in English.
- Give each page one primary Diátaxis type.
- Prefer project evidence over assumptions.
- Use simple, direct, consistent language and project terminology.
- Include examples that are complete, current, safe, and verified.
- Use existing documentation to learn tone and terminology. Do not copy its
  content unless the user explicitly requests reuse.
- Do not consult external sources unless the user provides a link and instructs
  you to use it.

## Classify documentation ownership first

Determine which files own the requested truth before drafting. Follow the
repository's declared ownership model; a common split is:

- entry-point README for concise discovery;
- contributor guide for development workflow;
- topical product and architecture pages for current behavior;
- specs and plans for design and execution history.

Historical specs and plans do not override current implementation or product
documentation. Do not describe planned behavior as shipped, and do not copy a
design document into product docs. When reviewing commits, treat commit
messages as navigation: verify claims from their diffs, tests, schemas, and
current code. Preserve explicit limitations and incomplete stages.

## Discover the project

Inspect the current project before designing the page:

1. Read the applicable repository instructions and contributor guides.
2. Locate the documentation workspace, `package.json`, `blume.config.ts`,
   content root, and `meta.ts` files.
3. Confirm the installed Blume version and read only the relevant documentation
   bundled with that package. From the documentation workspace, locate it with:

   ```bash
   node -e "console.log(require.resolve('blume/package.json'))"
   ```

4. Read nearby Markdown or MDX pages to learn navigation, tone, terminology,
   frontmatter, links, and component conventions.
5. Inspect the authoritative implementation sources for the requested topic:
   code, tests, schemas, generated specifications, CLI help, configuration, or
   current design decisions.
6. Inspect the working tree and preserve unrelated changes.

Follow local instructions when they define different paths, authorities, or
verification commands. Do not encode one project's layout as a universal Blume
rule.

## Follow the authoring workflow

### 1. Establish the documentation brief

Determine every required field before proposing the outline:

| Field | Required decision |
| --- | --- |
| Document type | Tutorial, how-to guide, reference, or explanation |
| Target audience | Reader role and relevant experience |
| Reader goal | The outcome or understanding the page must deliver |
| Included scope | Topics, paths, interfaces, or scenarios to cover |
| Excluded scope | Nearby topics the page must not absorb |

Use facts already supplied by the user or safely discovered in the project. Ask
one concise question at a time only when a missing decision would materially
change the result. Do not combine types such as “guide plus reference” to avoid
choosing a primary reader need.

For a new page or open-ended restructuring, restate the brief compactly for
confirmation. For maintenance explicitly requested after implementation or a
specified commit range, infer the brief from the request and authoritative diff
and proceed without adding an approval pause.

### 2. Build an evidence map

Map important claims to their authoritative project sources. Verify commands,
fields, defaults, states, transitions, failure behavior, and security
constraints before documenting them.

Use this working shape when the topic is complex:

| Claim area | Authority | Evidence still missing |
| --- | --- | --- |
| Public behavior | Tests, CLI help, API schema | Exact examples |
| Internal behavior | Production code, architecture docs | Ownership rationale |
| Operations | Runbooks, metrics, error contracts | Recovery limits |

Resolve contradictions in favor of the project's declared source of truth. If
the sources do not prove a claim, ask for the missing evidence or omit the
claim. Never turn a plausible guess into documentation.

### 3. Propose the outline when design is still open

Propose an outline containing:

- intended file path and page title;
- primary Diátaxis type, audience, and reader goal;
- each section heading with its purpose;
- examples, tables, diagrams, or callouts that materially help;
- navigation or `meta.ts` changes;
- evidence or user decisions still required.

Ask the user to approve or revise the outline for a new page or materially open
information architecture. Skip this gate for bounded maintenance where the
existing owner pages and implemented behavior determine the structure.

### 4. Draft the approved page

Write Blume-compatible Markdown or MDX in English:

- Add accurate `title` and `description` frontmatter. Follow the project's
  existing sidebar and SEO fields.
- Start page content at `##` when Blume renders the frontmatter title as the page
  heading.
- Use `.md` for plain Markdown and `.mdx` when the page needs Blume directives
  or components.
- Use built-in Blume components without imports. Use them only when they improve
  comprehension.
- Keep procedures in execution order and keep reference entries stable and
  scannable.
- Explain why only when it serves the chosen document type.
- Link instead of duplicating material owned by another page.
- Use reader-facing site routes for documentation links and source paths only
  for maintainer-facing instructions.
- Update navigation deliberately. When the project uses `meta.ts`, preserve its
  existing ordering model.

### 5. Verify before completion

Review the rendered content and its sources:

1. Confirm the page still serves one type, audience, and goal.
2. Recheck every command, code sample, state, field, default, link, and safety
   statement against current evidence.
3. Confirm headings, frontmatter, components, directives, and navigation match
   the installed Blume version.
4. Identify whether the changed files are inputs to the documentation site.
   For Blume inputs, run the workspace's formatting, link, doctor, and build
   commands. When standard npm scripts exist, run:

   ```bash
   npm run doctor
   npm run build
   ```

   For internal specs, plans, or other Markdown outside the Blume inputs, do
   not run Blume commands unless repository instructions explicitly require
   them; inspect the rendered Markdown and diff instead.

5. Run every additional repository-required check and inspect the complete diff.

Report content failures separately from missing dependencies or environmental
blockers. Do not claim completion while a required check fails.

## Choose one Diátaxis format

| Type | Reader need | Required sections | Keep out |
| --- | --- | --- | --- |
| Tutorial | Learn by completing a guided outcome | Outcome, prerequisites, sequential lesson, checkpoints, result, next steps | Exhaustive options and branching recovery |
| How-to guide | Solve a specific problem | Goal, prerequisites, procedure, verification, troubleshooting or recovery | Conceptual tours and unrelated alternatives |
| Reference | Look up exact facts | Scope, stable entries, parameters or fields, constraints, examples, related reference | Narrative teaching and operational walkthroughs |
| Explanation | Understand a concept or decision | Context, conceptual model, relationships, trade-offs, consequences, related topics | Step-by-step task instructions |

Treat architecture and workflow as subject-specific overlays, not additional
Diátaxis types.

## Structure architecture documentation

Add the relevant sections below to the primary Diátaxis format:

1. **Purpose, scope, and non-goals** — define what the system owns.
2. **System context and boundaries** — identify users, external systems, and
   trust boundaries.
3. **Components and responsibilities** — name each component, owner, inputs,
   outputs, and non-responsibilities.
4. **Dependencies and communication** — distinguish synchronous calls,
   asynchronous messages, and persistence boundaries.
5. **Data or state ownership and lifecycle** — show authoritative state,
   transitions, retention, and deletion.
6. **Important flows** — trace normal and failure sequences across components.
7. **Invariants and security constraints** — state guarantees, authorization,
   isolation, validation, and sensitive-data rules.
8. **Failure, recovery, and observability** — connect failures to detection,
   automatic response, operator action, and durable outcome.
9. **Trade-offs and extension points** — explain decisions, limitations, and
   safe ways to extend the system.
10. **Related documentation** — link to reference, operations, and workflow
    pages without duplicating them.

Use a diagram only when it makes a multi-component relationship or sequence
materially clearer. Keep the prose authoritative and describe the same critical
facts accessibly in text.

## Structure workflow documentation

Add the relevant sections below to the primary Diátaxis format:

1. **Goal and completion condition** — define the observable successful result.
2. **Actors and ownership** — distinguish user actions, automated behavior, and
   the component that owns each transition.
3. **Trigger and prerequisites** — state when the workflow applies and what must
   already be true.
4. **Ordered happy path** — present actions and system responses in execution
   order.
5. **State transitions and durable side effects** — name old state, new state,
   owner, persistence, and external effects.
6. **Validation and runtime failures** — distinguish rejected input from work
   that started and failed.
7. **Retries, recovery, cancellation, and rollback** — state safe boundaries and
   what happens after interruption or an unknown result.
8. **Concurrency, ordering, and idempotency** — document duplicate requests,
   locks, races, and replay safety when relevant.
9. **Observability and operator actions** — name logs, events, metrics, audit
   evidence, escalation, and safe interventions.
10. **Verification** — prove the final state and important side effects.

## Example: brief to Blume page

This example demonstrates structure only. Replace every project behavior with
facts verified in the target project.

**Confirmed brief**

- Type: how-to guide
- Audience: experienced on-call operators
- Goal: safely replay one failed job
- Include: preflight, replay, state changes, recovery, verification
- Exclude: architecture background and bulk replay

**Approved outline**

1. Outcome and scope
2. Before you begin
3. Inspect the failed job
4. Replay one job
5. Expected state changes
6. Verify the result
7. Recover from an incomplete replay

**Representative MDX shape**

```mdx
---
title: Replay one failed job
description: Safely replay a single failed job and verify its final state.
---

## Before you begin

Confirm that the original failure is resolved and that no worker is processing
the job.

:::warning[Prevent duplicate effects]
Query the authoritative job state after a timeout before attempting another
replay.
:::

## Replay the job

<Steps>
  <Step title="Inspect the current state">
    Continue only when the project-defined replay eligibility rules pass.
  </Step>
  <Step title="Run the verified single-job command">
    Use the exact command and arguments confirmed from the current CLI.
  </Step>
  <Step title="Verify completion">
    Confirm the final state and every externally visible side effect.
  </Step>
</Steps>
```

## Quick reference

| Stage | Required output |
| --- | --- |
| Brief | Type, audience, goal, included scope, excluded scope |
| Research | Claims mapped to current project evidence |
| Outline | Path, sections and purposes, examples, navigation, open evidence |
| Draft | English Blume Markdown or MDX following one primary type |
| Verification | Checked claims, examples, links, navigation, doctor, build, repo gates |

## Common mistakes

| Mistake | Correction |
| --- | --- |
| Mixing tutorial, how-to, reference, and explanation structures | Choose the reader's primary need and link to the other types |
| Asking questions the project already answers | Inspect local sources first, then ask only for missing decisions |
| Drafting while the brief or outline is unapproved | Stop at the current gate and request confirmation |
| Copying nearby pages | Reuse terminology and conventions, not prose |
| Inventing commands, states, defaults, or guarantees | Verify them from authoritative project evidence |
| Assuming every Blume project has the same layout | Discover the installed version, config, content root, and navigation |
| Using components decoratively | Prefer the smallest structure that improves comprehension |
| Treating a successful build as proof of factual accuracy | Verify claims and examples separately from syntax and build checks |
