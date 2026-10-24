import { useState } from "react"
import { useApi } from "@/components/apiContext"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import type { WorkflowArtifact, WorkflowDeclaredArtifact } from "@/lib/api"
import { cn } from "@/lib/utils"
import { COUNT, DANGER_FILL, EMPTY, LABEL, MONO, QUIET_ACTION } from "./panelStyles"
import { MarkdownContent, MarkdownModeSegment, type MarkdownMode } from "./TaskMarkdown"
import { formatTaskTime } from "./taskTime"
import type { Requirement, StepArtifacts } from "./workflowGraph"
import { errorText } from "./workflowShared"

/** A value longer than this many lines, or characters, starts collapsed. */
const COLLAPSED_LINES = 6
const COLLAPSED_CHARS = 600

/**
 * The task's artifacts: the current value of each, its author and time, the
 * history on demand, and for the customer an edit action. The daemon never
 * parses a value: it renders as Markdown, or as plain text when switched.
 * Given a step of the workflow, only the artifacts a transition into or out
 * of it requires show, in those two groups, until **Show all artifacts**;
 * one needed both ways shows once, under the way in.
 * Otherwise every artifact shows in one list, the ones an outcome of the
 * current status needs first; in both, missing ones come before set ones and
 * each names the transition that requires it.
 */
export default function WorkflowArtifacts({ taskKey, artifacts, declared, editable, required, step, onShowAll, onChanged }: {
  taskKey: string
  artifacts: WorkflowArtifact[]
  /** Every artifact the manifest declares, so one with no value can be set. */
  declared: WorkflowDeclaredArtifact[]
  editable: boolean
  /** The outcomes of the current status that require each artifact, by name. */
  required?: Map<string, Requirement[]>
  /** The selected step and what it needs; absent shows every artifact. */
  step?: { status: string } & StepArtifacts
  onShowAll?: () => void
  onChanged: () => void
}) {
  const unset = declared.filter((item) => !artifacts.some((artifact) => artifact.name === item.name))
  const describe = (name: string) => declared.find((item) => item.name === name)?.description
  const value = (name: string) => artifacts.find((artifact) => artifact.name === name)
  const item = (name: string, requirements?: Requirement[]) => <ArtifactItem key={name} taskKey={taskKey} name={name}
    artifact={value(name)} description={describe(name)} requirements={requirements} editable={editable} onChanged={onChanged} />
  const all = [...artifacts.map((artifact) => artifact.name), ...unset.map(({ name }) => name)]
  // Missing first: what blocks the transition is what to look at.
  const missingFirst = (names: string[]) => [...names].sort((a, b) => Number(!!value(a)) - Number(!!value(b)))
  const header = <div className="flex items-center gap-1.5">
    <span className={LABEL}>Artifacts</span>
    <span className={COUNT}>{artifacts.length}</span>
  </div>
  if (step) {
    const shown = new Set([...step.enter.keys(), ...step.leave.keys()])
    // An artifact needed both ways shows once, under the way in, with every label.
    const both = (name: string) => [...step.enter.get(name) ?? [], ...step.leave.get(name) ?? []]
    const group = (title: string, names: string[]) => names.length > 0 && <>
      <span className="text-[11.5px] text-muted-foreground">{title}</span>
      <ul aria-label={title} className="flex min-w-0 flex-col gap-2.5">
        {missingFirst(names).map((name) => item(name, both(name)))}
      </ul>
    </>
    return (
      <section className="flex min-w-0 flex-col gap-1.5">
        {header}
        <div data-testid="artifact-filter" className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[12px] text-muted-foreground">
          <span className="min-w-0">Step <span className={cn(MONO, "break-all text-foreground")}>{step.status}</span> · {shown.size} of {all.length} artifacts</span>
          <Button type="button" variant="ghost" className={QUIET_ACTION} onClick={onShowAll}>Show all artifacts</Button>
        </div>
        {shown.size === 0 && <span className={EMPTY}>No artifact is required to enter or leave {step.status}.</span>}
        {group(`Needed to enter ${step.status}`, [...step.enter.keys()])}
        {group(`Needed to leave ${step.status}`, [...step.leave.keys()].filter((name) => !step.enter.has(name)))}
      </section>
    )
  }
  const needed = missingFirst(all.filter((name) => required?.has(name)))
  return (
    <section className="flex min-w-0 flex-col gap-1.5">
      {header}
      {all.length === 0 && <span className={EMPTY}>No artifact has a value yet.</span>}
      {all.length > 0 && <ul aria-label="All artifacts" className="flex min-w-0 flex-col gap-2.5">
        {[...needed, ...all.filter((name) => !required?.has(name))].map((name) => item(name, required?.get(name)))}
      </ul>}
    </section>
  )
}

function ArtifactItem({ taskKey, name, artifact, description, requirements, editable, onChanged }: {
  taskKey: string
  name: string
  artifact?: WorkflowArtifact
  /** The manifest's description. */
  description?: string
  /** The transitions that require the artifact. */
  requirements?: Requirement[]
  editable: boolean
  onChanged: () => void
}) {
  const api = useApi()
  const [draft, setDraft] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState("")
  const [history, setHistory] = useState<WorkflowArtifact[] | null>(null)
  const [historyError, setHistoryError] = useState("")
  const [mode, setMode] = useState<MarkdownMode>("rich")
  const save = () => {
    if (draft === null) return
    setSaving(true)
    setError("")
    api.workflows.setTaskArtifact(taskKey, name, draft)
      .then(() => { setDraft(null); setHistory(null); onChanged() })
      .catch((failure) => setError(errorText(failure)))
      .finally(() => setSaving(false))
  }
  const toggleHistory = () => {
    if (history) { setHistory(null); return }
    setHistoryError("")
    api.workflows.getTaskArtifactHistory(taskKey, name)
      .then(setHistory)
      .catch((failure) => setHistoryError(errorText(failure)))
  }
  return (
    <li className="flex min-w-0 flex-col gap-1">
      <div className="flex min-w-0 items-center gap-2">
        <span className={cn(MONO, "font-medium")}>{name}</span>
        {artifact ? <>
          <span className={cn(MONO, "min-w-0 break-all text-muted-foreground")}>{artifact.author}</span>
          <time dateTime={artifact.created_at} className={cn(MONO, "text-muted-foreground")}>{formatTaskTime(artifact.created_at)}</time>
        </> : <span className={EMPTY}>no value</span>}
        <span className="ml-auto flex items-center gap-1">
          {artifact && draft === null && <MarkdownModeSegment label={`Show ${name} as`} names={["Markdown", "Text"]}
            mode={mode} onModeChange={setMode} />}
          {artifact && <Button type="button" variant="ghost" className={QUIET_ACTION} aria-expanded={history !== null}
            onClick={toggleHistory}>History</Button>}
          {editable && draft === null && <Button type="button" variant="ghost" className={QUIET_ACTION}
            onClick={() => setDraft(artifact?.value ?? "")}>{artifact ? "Edit" : "Set"}</Button>}
        </span>
      </div>
      {(description || (requirements ?? []).length > 0) && <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5">
        {description && <span className="min-w-0 text-[12px] break-words text-muted-foreground">{description}</span>}
        {(requirements ?? []).map((requirement) => <span key={`${requirement.from}:${requirement.on}`} data-requirement={requirement.missing ? "missing" : "set"}
          className={cn(MONO, "rounded-[5px] px-1.5", requirement.missing ? DANGER_FILL : "bg-muted text-muted-foreground")}>
          required by {requirement.from && `${requirement.from} · `}{requirement.on} → {requirement.to}</span>)}
      </div>}
      {draft !== null ? <div className="flex min-w-0 flex-col gap-1.5">
        <Textarea aria-label={`Value of ${name}`} value={draft} disabled={saving} onChange={(event) => setDraft(event.target.value)}
          className="min-h-20 rounded-[8px] border-0 bg-muted font-mono text-[11.5px] md:text-[11.5px]" />
        <div className="flex gap-1.5">
          <Button type="button" size="sm" className="h-7" disabled={saving || !draft} onClick={save}>Save</Button>
          <Button type="button" variant="ghost" size="sm" className="h-7" disabled={saving}
            onClick={() => { setDraft(null); setError("") }}>Discard</Button>
        </div>
      </div> : artifact && <ArtifactValue value={artifact.value} markdown={mode === "rich"} />}
      {error && <p role="alert" className="text-[12px] text-status-failed">{error}</p>}
      {historyError && <p role="alert" className="text-[12px] text-status-failed">{historyError}</p>}
      {history && <ol className="flex min-w-0 flex-col gap-1.5 border-l border-border pl-2.5">
        {history.map((entry) => <li key={entry.id} className="flex min-w-0 flex-col gap-0.5">
          <span className={cn(MONO, "text-muted-foreground")}>{entry.author} · {formatTaskTime(entry.created_at)}</span>
          <ArtifactValue value={entry.value} markdown={mode === "rich"} />
        </li>)}
      </ol>}
    </li>
  )
}

function ArtifactValue({ value, markdown }: { value: string, markdown: boolean }) {
  const [expanded, setExpanded] = useState(false)
  const lines = value.split("\n")
  const manyLines = lines.length > COLLAPSED_LINES
  const short = lines.slice(0, COLLAPSED_LINES).join("\n").slice(0, COLLAPSED_CHARS)
  const long = short.length < value.length
  const shown = long && !expanded ? short : value
  return <div className="flex min-w-0 flex-col items-start gap-0.5">
    {markdown
      ? <div className="w-full min-w-0 rounded-[8px] bg-muted px-2.5 py-2"><MarkdownContent>{shown}</MarkdownContent></div>
      : <pre className="w-full min-w-0 rounded-[8px] bg-muted px-2.5 py-2 font-mono text-[11.5px] whitespace-pre-wrap break-words">{shown}</pre>}
    {long && <Button type="button" variant="ghost" className={QUIET_ACTION} aria-expanded={expanded}
      onClick={() => setExpanded(!expanded)}>
      {expanded ? "Show less" : manyLines ? `Show all ${lines.length} lines` : `Show all ${value.length} characters`}
    </Button>}
  </div>
}
