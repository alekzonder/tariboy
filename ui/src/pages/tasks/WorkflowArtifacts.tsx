import { useState } from "react"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import type { ApiTarget } from "@/lib/api"
import { getTaskArtifactHistory, setTaskArtifact, type WorkflowArtifact, type WorkflowDeclaredArtifact } from "@/lib/tasks"
import { cn } from "@/lib/utils"
import { COUNT, DANGER_FILL, EMPTY, LABEL, MONO, QUIET_ACTION } from "./panelStyles"
import { MarkdownContent, MarkdownModeSegment, type MarkdownMode } from "./TaskMarkdown"
import { formatTaskTime } from "./taskTime"
import type { Requirement } from "./workflowGraph"
import { errorText } from "./workflowShared"

/** A value longer than this many lines, or characters, starts collapsed. */
const COLLAPSED_LINES = 6
const COLLAPSED_CHARS = 600

/**
 * The task's artifacts: the current value of each, its author and time, the
 * history on demand, and for the customer an edit action. The daemon never
 * parses a value: it renders as Markdown, or as plain text when switched.
 * Given the requirements of the current status, the artifacts an outcome
 * needs come first, missing ones before set ones, each naming the outcome;
 * the rest fold away until asked for.
 */
export default function WorkflowArtifacts({ taskKey, artifacts, declared, editable, required, target, onChanged }: {
  taskKey: string
  artifacts: WorkflowArtifact[]
  /** Every artifact the manifest declares, so one with no value can be set. */
  declared: WorkflowDeclaredArtifact[]
  editable: boolean
  /** The outcomes of the current status that require each artifact, by name. */
  required?: Map<string, Requirement[]>
  target?: ApiTarget
  onChanged: () => void
}) {
  const [othersOpen, setOthersOpen] = useState(false)
  const unset = declared.filter((item) => !artifacts.some((artifact) => artifact.name === item.name))
  const describe = (name: string) => declared.find((item) => item.name === name)?.description
  const item = (name: string, artifact?: WorkflowArtifact) => <ArtifactItem key={name} taskKey={taskKey} name={name}
    artifact={artifact} description={describe(name)} requirements={required?.get(name)} editable={editable} target={target} onChanged={onChanged} />
  const all: { name: string; artifact?: WorkflowArtifact }[] = [
    ...artifacts.map((artifact) => ({ name: artifact.name, artifact })), ...unset.map(({ name }) => ({ name }))]
  const needed = all.filter(({ name }) => required?.has(name))
  const others = all.filter(({ name }) => !required?.has(name))
  // Missing first: what blocks the outcome is what to look at.
  needed.sort((a, b) => Number(!!a.artifact) - Number(!!b.artifact))
  const grouped = needed.length > 0
  return (
    <section className="flex min-w-0 flex-col gap-1.5">
      <div className="flex items-center gap-1.5">
        <span className={LABEL}>Artifacts</span>
        <span className={COUNT}>{artifacts.length}</span>
      </div>
      {all.length === 0 && <span className={EMPTY}>No artifact has a value yet.</span>}
      {grouped && <>
        <span className="text-[11.5px] text-muted-foreground">Needed to leave this status</span>
        <ul aria-label="Artifacts needed now" className="flex min-w-0 flex-col gap-2.5">
          {needed.map(({ name, artifact }) => item(name, artifact))}
        </ul>
        {others.length > 0 && <Button type="button" variant="ghost" className={cn(QUIET_ACTION, "self-start")} aria-expanded={othersOpen}
          onClick={() => setOthersOpen(!othersOpen)}>
          {othersOpen ? "Hide other artifacts" : `Other artifacts (${others.length})`}</Button>}
      </>}
      {(!grouped || othersOpen) && others.length > 0 && <ul aria-label="Other artifacts" className="flex min-w-0 flex-col gap-2.5">
        {others.map(({ name, artifact }) => item(name, artifact))}
      </ul>}
    </section>
  )
}

function ArtifactItem({ taskKey, name, artifact, description, requirements, editable, target, onChanged }: {
  taskKey: string
  name: string
  artifact?: WorkflowArtifact
  /** The manifest's description. */
  description?: string
  /** The outcomes of the current status that require the artifact. */
  requirements?: Requirement[]
  editable: boolean
  target?: ApiTarget
  onChanged: () => void
}) {
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
    setTaskArtifact(taskKey, name, draft, target)
      .then(() => { setDraft(null); setHistory(null); onChanged() })
      .catch((failure) => setError(errorText(failure)))
      .finally(() => setSaving(false))
  }
  const toggleHistory = () => {
    if (history) { setHistory(null); return }
    setHistoryError("")
    getTaskArtifactHistory(taskKey, name, target)
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
        {(requirements ?? []).map((requirement) => <span key={requirement.on} data-requirement={requirement.missing ? "missing" : "set"}
          className={cn(MONO, "rounded-[5px] px-1.5", requirement.missing ? DANGER_FILL : "bg-muted text-muted-foreground")}>
          required by {requirement.on} → {requirement.to}</span>)}
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
