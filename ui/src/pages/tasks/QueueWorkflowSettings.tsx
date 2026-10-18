import { useCallback, useEffect, useMemo, useState } from "react"
import { Link } from "react-router-dom"
import { Button } from "@/components/ui/button"
import { ApiError, type ApiTarget } from "@/lib/api"
import {
  clearQueueWorkflow, getQueueWorkflow, getWorkflowImage, listWorkflowImages, setQueueWorkflow,
  type AgentPool, type QueueWorkflow, type WorkflowDefinition, type WorkflowImage,
} from "@/lib/tasks"
import { EMPTY, LABEL, MONO, SelectShell } from "./panelStyles"
import { useWorkflowConfirm } from "./WorkflowConfirm"
import WorkflowStatusMap from "./WorkflowStatusMap"
import { errorText } from "./workflowShared"

function detailList(error: ApiError, name: string): string[] {
  const list = error.details?.[name]
  return Array.isArray(list) ? list.map(String) : []
}

/** Images by name, then tag with `latest` first. */
function sortImages(images: WorkflowImage[]): WorkflowImage[] {
  const rank = (image: WorkflowImage) => (image.tag === "latest" ? 0 : 1)
  return [...images].sort((a, b) => a.name.localeCompare(b.name) || rank(a) - rank(b) || a.tag.localeCompare(b.tag))
}

/** The tag of the bound content, the one equal to its version first; the digest when no tag names it. */
function bindingRef(binding: QueueWorkflow, images: WorkflowImage[]): string {
  const tags = images.filter((image) => image.name === binding.name && image.digest === binding.digest)
  return (tags.find((image) => image.tag === binding.version) ?? tags[0])?.tag ?? binding.digest
}

/**
 * The Workflow tab of a queue: the bound image, a select to bind another, and
 * the statuses of the bound or the chosen image with the pool that owns each.
 * A bind refused for empty pools or missing secrets lists them with a way to
 * the tab that fixes them; the pools list shrinks as `pools` gains agents.
 */
export default function QueueWorkflowSettings({ queue, target, pools, onOpenTab, onPoolsMissing, onSecretsMissing, onChanged }: {
  queue: string
  target?: ApiTarget
  /** The queue's pools, kept current by the Pools tab. */
  pools: AgentPool[]
  onOpenTab: (tab: "pools" | "secrets") => void
  /** Told of the pools a refused bind named, and of null when a new attempt starts. */
  onPoolsMissing: (names: string[] | null) => void
  /** Told of the secrets a refused bind named, and of null when a new attempt starts. */
  onSecretsMissing: (missing: { names: string[]; message: string } | null) => void
  /** Told after the binding changed here. */
  onChanged: () => void
}) {
  const [binding, setBinding] = useState<QueueWorkflow | null>(null)
  // True only after a load succeeded: until then the binding is unknown.
  const [loaded, setLoaded] = useState(false)
  const [loadError, setLoadError] = useState("")
  const [images, setImages] = useState<WorkflowImage[]>([])
  const [ref, setRef] = useState("")
  const [error, setError] = useState("")
  const [notice, setNotice] = useState("")
  const [emptyPools, setEmptyPools] = useState<{ names: string[]; message: string } | null>(null)
  const [missingSecrets, setMissingSecrets] = useState<{ names: string[]; message: string } | null>(null)
  const [definition, setDefinition] = useState<{ key: string; value: WorkflowDefinition | null; error: string }>({ key: "", value: null, error: "" })
  const [busy, setBusy] = useState(false)
  const { confirm, dialog } = useWorkflowConfirm()

  const loadBinding = useCallback(async () => {
    try {
      setBinding(await getQueueWorkflow(queue, target))
      setLoaded(true)
      setLoadError("")
    } catch (err) {
      setLoaded(false)
      setLoadError(errorText(err))
    }
  }, [queue, target])
  useEffect(() => { void Promise.resolve().then(loadBinding) }, [loadBinding])
  useEffect(() => {
    void listWorkflowImages(target).then(setImages, (err) => setError(errorText(err)))
  }, [target])
  const sorted = useMemo(() => sortImages(images), [images])

  const refused = async (err: unknown) => {
    if (err instanceof ApiError && err.code === "revision_conflict") {
      await loadBinding()
      setNotice("The binding changed elsewhere. It was reloaded; review and retry.")
    } else if (err instanceof ApiError && err.code === "workflow_pool_empty") {
      setEmptyPools({ names: detailList(err, "pools"), message: err.message })
      onPoolsMissing(detailList(err, "pools"))
    } else if (err instanceof ApiError && err.code === "workflow_secret_missing") {
      const missing = { names: detailList(err, "secrets"), message: err.message }
      setMissingSecrets(missing)
      onSecretsMissing(missing)
    } else setError(errorText(err))
  }

  const reset = () => { setError(""); setNotice(""); setEmptyPools(null); setMissingSecrets(null); onPoolsMissing(null); onSecretsMissing(null) }
  const stillEmpty = emptyPools?.names.filter((name) => !pools.find((pool) => pool.name === name)?.agents.length) ?? []

  // The chosen image previews its statuses; otherwise the bound one shows, as NAME:DIGEST.
  const shownKey = ref || (binding ? `${binding.name}:${binding.digest}` : "")
  useEffect(() => {
    if (!shownKey) return
    let alive = true
    const colon = shownKey.lastIndexOf(":")
    getWorkflowImage(shownKey.slice(0, colon), shownKey.slice(colon + 1), target).then(
      (manifest) => { if (alive) setDefinition({ key: shownKey, value: manifest.definition, error: "" }) },
      (err) => { if (alive) setDefinition({ key: shownKey, value: null, error: errorText(err) }) },
    )
    return () => { alive = false }
  }, [shownKey, target])

  const bind = async () => {
    reset()
    setBusy(true)
    try {
      setBinding(await setQueueWorkflow(queue, ref, binding?.revision ?? 0, target))
      setRef("")
      onChanged()
    } catch (err) { await refused(err) } finally { setBusy(false) }
  }

  const clear = () => {
    if (!binding) return
    confirm({
      title: `Clear the workflow of ${queue}?`,
      description: "New tasks in this queue become flexible tasks. Tasks already running the workflow keep it.",
      action: "Clear binding",
      run: () => {
        reset()
        setBusy(true)
        clearQueueWorkflow(queue, binding.revision, target).then(() => { setBinding(null); onChanged() }, refused).finally(() => setBusy(false))
      },
    })
  }

  return (
    <section className="flex min-w-0 flex-col gap-2.5" aria-label={`Workflow ${queue}`}>
      <span className={LABEL}>Workflow</span>
      {loaded && (binding ? (
        <p data-testid="binding" className="text-[12.5px]">
          {/* The queue settings live at /servers/:hostId/tasks; the image page is a sibling section. */}
          <Link relative="path" className="font-semibold text-primary hover:underline"
            to={`../workflows/${encodeURIComponent(binding.name)}/${encodeURIComponent(bindingRef(binding, images))}`}>
            {binding.name}
          </Link> {binding.version}{" "}
          <span className={MONO} title={binding.digest}>{binding.digest.slice(0, 12)}</span>
        </p>
      ) : <span className={EMPTY}>No workflow</span>)}
      {loadError && (
        <div role="alert" className="flex items-center gap-2 text-[12px] text-destructive">
          <span>{loadError}</span>
          <Button type="button" size="sm" variant="secondary" className="h-[26px]" onClick={() => void loadBinding()}>Retry</Button>
        </div>
      )}
      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
        <SelectShell aria-label="Workflow image" value={ref} className="h-[26px] w-auto text-[12px] md:text-[12px]"
          onChange={(event) => setRef(event.target.value)}>
          <option value="">Choose an image</option>
          {sorted.map((image) => <option key={`${image.name}:${image.tag}`} value={`${image.name}:${image.tag}`}>{image.name}:{image.tag}</option>)}
        </SelectShell>
        <Button type="button" size="sm" className="h-[26px]" disabled={busy || !ref || !loaded} onClick={() => void bind()}>Bind</Button>
        <Button type="button" size="sm" variant="secondary" className="h-[26px]" disabled={busy || !loaded || !binding} onClick={clear}>Clear</Button>
      </div>
      {notice && <p role="status" className="text-[12px] text-muted-foreground">{notice}</p>}
      {error && <p role="alert" className="text-[12px] text-destructive">{error}</p>}
      {emptyPools && stillEmpty.length > 0 && (
        <div role="alert" aria-label="Pools needed" className="flex flex-col gap-1 text-[12px] text-destructive">
          <p>{emptyPools.message}</p>
          <p className="font-mono">{stillEmpty.join(", ")}</p>
          <Button type="button" size="sm" variant="secondary" className="h-[26px] self-start" onClick={() => onOpenTab("pools")}>Open Pools</Button>
        </div>
      )}
      {missingSecrets && (
        <div role="alert" aria-label="Secrets needed" className="flex flex-col gap-1 text-[12px] text-destructive">
          <p>{missingSecrets.message}</p>
          <p className="font-mono">{missingSecrets.names.join(", ")}</p>
          <Button type="button" size="sm" variant="secondary" className="h-[26px] self-start" onClick={() => onOpenTab("secrets")}>Open Secrets</Button>
        </div>
      )}
      {shownKey && <section className="flex min-w-0 flex-col gap-1.5">
        <span className={LABEL}>{ref ? `Statuses of ${ref}` : "Statuses"}</span>
        {definition.key !== shownKey ? <span className={EMPTY}>Loading statuses…</span>
          : definition.value ? <WorkflowStatusMap definition={definition.value} pools={pools} onPool={() => onOpenTab("pools")} />
            : <p role="alert" className="text-[12px] text-destructive">{definition.error}</p>}
      </section>}
      {dialog}
    </section>
  )
}
