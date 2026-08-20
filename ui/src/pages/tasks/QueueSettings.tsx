import { useCallback, useEffect, useState } from "react"
import { toast } from "sonner"
import { ApiError, type ApiTarget } from "@/lib/api"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import {
  activateQueueWorkflow,
  createWorkflowDraft,
  getQueueWorkflow,
  listAgentPools,
  listWorkflowVersions,
  publishWorkflowVersion,
  rebindAgentPool,
  type AgentPool,
  type CreateQueueInput,
  type QueueWorkflowBinding,
  type TaskQueue,
  type UpdateQueueInput,
  type WorkflowDefinition,
  type WorkflowVersion,
} from "@/lib/tasks"

export default function QueueSettings({
  queues,
  onCreate,
  onUpdate,
  target,
}: {
  queues: TaskQueue[]
  onCreate: (input: CreateQueueInput) => Promise<void>
  onUpdate: (prefix: string, input: UpdateQueueInput) => Promise<void>
  target?: ApiTarget
}) {
  const [creating, setCreating] = useState(false)
  const [prefix, setPrefix] = useState("")
  const [name, setName] = useState("")
  const [owners, setOwners] = useState("")
  const [responsible, setResponsible] = useState("")

  const submit = async (event: React.FormEvent) => {
    event.preventDefault()
    await onCreate({
      prefix: prefix.trim().toUpperCase(),
      name: name.trim(),
      owners: owners.split(",").map((owner) => owner.trim()).filter(Boolean),
      responsible_agent: responsible.trim(),
    })
    setCreating(false)
    setPrefix("")
    setName("")
    setOwners("")
    setResponsible("")
  }

  return (
    <section className="task-queues">
      <header><h2>Queues</h2><button type="button" onClick={() => setCreating(true)}>New queue</button></header>
      {creating && (
        <form onSubmit={(event) => void submit(event)}>
          <label>Queue prefix<Input aria-label="Queue prefix" value={prefix} onChange={(event) => setPrefix(event.target.value)} /></label>
          <label>Queue name<Input aria-label="New queue name" value={name} onChange={(event) => setName(event.target.value)} /></label>
          <label>Owner agents<Input value={owners} placeholder="alice, agent:bob" onChange={(event) => setOwners(event.target.value)} /></label>
          <label>Responsible agent<Input value={responsible} onChange={(event) => setResponsible(event.target.value)} /></label>
          <div><button type="submit" disabled={!prefix.trim() || !name.trim()}>Create queue</button><button type="button" onClick={() => setCreating(false)}>Cancel</button></div>
        </form>
      )}
      <div className="task-queue-list">
        {queues.map((queue) => (
          <article key={`${queue.prefix}:${queue.revision}`}>
            <strong>{queue.prefix}</strong><h3>{queue.name}</h3>
            <form onSubmit={(event) => {
              event.preventDefault()
              const values = new FormData(event.currentTarget)
              void onUpdate(queue.prefix, {
                name: String(values.get("name") ?? "").trim(),
                description: String(values.get("description") ?? "").trim(),
                owners: String(values.get("owners") ?? "").split(",").map((value) => value.trim()).filter(Boolean),
                responsible_agent: String(values.get("responsible") ?? "").trim(),
                revision: queue.revision,
              })
            }}>
              <label>Queue name<Input name="name" aria-label={`Queue name ${queue.prefix}`} defaultValue={queue.name} /></label>
              <label>Description<Textarea name="description" aria-label={`Queue description ${queue.prefix}`} defaultValue={queue.description} /></label>
              <label>Owner agents<Input name="owners" aria-label={`Queue owners ${queue.prefix}`} defaultValue={queue.owners.join(", ")} /></label>
              <label>Responsible agent<Input name="responsible" aria-label={`Queue triager ${queue.prefix}`} defaultValue={queue.responsible_agent} /></label>
              <button type="submit">Save {queue.prefix}</button>
            </form>
            <QueueWorkflowEditor queue={queue} target={target} />
          </article>
        ))}
      </div>
    </section>
  )
}

function actionKey(prefix: string): string {
  return globalThis.crypto?.randomUUID?.() ?? `${prefix}-${Date.now()}-${Math.random()}`
}

function QueueWorkflowEditor({ queue, target }: { queue: TaskQueue; target?: ApiTarget }) {
  const [binding, setBinding] = useState<QueueWorkflowBinding | null>(null)
  const [versions, setVersions] = useState<WorkflowVersion[]>([])
  const [pools, setPools] = useState<AgentPool[]>([])
  const [workflowName, setWorkflowName] = useState("")
  const [selectedVersion, setSelectedVersion] = useState("")
  const [poolName, setPoolName] = useState("")
  const [poolAgents, setPoolAgents] = useState("")
  const [definition, setDefinition] = useState("")
  const [busy, setBusy] = useState(false)
  const [stateLoading, setStateLoading] = useState(true)
  const [stateError, setStateError] = useState("")
  const [staleMessage, setStaleMessage] = useState("")

  const loadState = useCallback(async () => {
    setStateLoading(true)
    setStateError("")
    const [bound, poolPage] = await Promise.allSettled([
      getQueueWorkflow(queue.prefix, target), listAgentPools(queue.prefix, target),
    ])
    const bindingMissing = bound.status === "rejected" && bound.reason instanceof ApiError &&
      bound.reason.status === 404 && ["workflow_not_found", "queue_workflow_not_found"].includes(bound.reason.code)
    const errors = [
      bound.status === "rejected" && !bindingMissing ? bound.reason : null,
      poolPage.status === "rejected" ? poolPage.reason : null,
    ].filter(Boolean)
    if (errors.length > 0) {
      setStateError(errors.map((error) => error instanceof Error ? error.message : String(error)).join("; "))
    } else {
      if (bound.status === "fulfilled") {
        setBinding(bound.value)
        setWorkflowName(bound.value.workflow_name ?? "")
      } else setBinding(null)
      if (poolPage.status === "fulfilled") setPools(poolPage.value.items ?? [])
    }
    setStateLoading(false)
  }, [queue.prefix, target])

  useEffect(() => { void Promise.resolve().then(loadState) }, [loadState])

  const protect = async (operation: () => Promise<void>) => {
    setBusy(true)
    setStaleMessage("")
    try { await operation() } catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      if (typeof error === "object" && error && "status" in error && error.status === 409) {
        await loadState()
        setStaleMessage("Configuration changed elsewhere. Current revisions were reloaded; review and retry.")
      }
      toast.error(message)
    } finally { setBusy(false) }
  }

  return <section className="task-queue-workflow" aria-label={`Workflow ${queue.prefix}`}>
    <h4>Workflow</h4>
    {stateLoading ? <p>Loading workflow state…</p> : stateError ? <div role="alert" aria-label={`Workflow ${queue.prefix} error`}><p>{stateError}</p><button type="button" aria-label={`Retry workflow state ${queue.prefix}`} onClick={() => void loadState()}>Retry</button></div> : <p>{binding ? `Active: ${binding.workflow_name}@${binding.workflow_version} · rev ${binding.revision}` : "Legacy queue (no workflow)"}</p>}
    {staleMessage && <p role="alert">{staleMessage}</p>}
    <fieldset disabled={stateLoading || Boolean(stateError)}>
    <form onSubmit={(event) => { event.preventDefault(); void protect(async () => {
      const page = await listWorkflowVersions(workflowName.trim(), target)
      setVersions(page.items.filter((item) => item.state === "published"))
    }) }}>
      <label>Workflow name<Input aria-label={`Workflow name ${queue.prefix}`} value={workflowName} onChange={(event) => setWorkflowName(event.target.value)} /></label>
      <button type="submit" disabled={busy || !workflowName.trim()}>Load versions</button>
    </form>
    {versions.length > 0 && <form onSubmit={(event) => { event.preventDefault(); void protect(async () => {
      const next = await activateQueueWorkflow(queue.prefix, Number(selectedVersion), binding?.revision ?? 0, actionKey("activate"), target)
      setBinding(next)
      toast.success("Workflow activated")
    }) }}>
      <label>Published version<select aria-label={`Published workflow version ${queue.prefix}`} value={selectedVersion} onChange={(event) => setSelectedVersion(event.target.value)}><option value="">Select…</option>{versions.map((item) => <option key={item.id} value={item.id}>{item.name}@{item.version}</option>)}</select></label>
      <button type="submit" disabled={busy || !selectedVersion}>Activate workflow</button>
    </form>}
    <details><summary>Create definition (JSON)</summary><form onSubmit={(event) => { event.preventDefault(); void protect(async () => {
      const parsed = JSON.parse(definition) as WorkflowDefinition
      const draft = await createWorkflowDraft(parsed, target)
      await publishWorkflowVersion(draft.name, draft.version, target)
      setWorkflowName(draft.name)
      setVersions((current) => [...current.filter((item) => item.id !== draft.id), { ...draft, state: "published" }])
      toast.success("Workflow published")
    }) }}><label>Workflow definition<Textarea aria-label={`Workflow definition ${queue.prefix}`} value={definition} onChange={(event) => setDefinition(event.target.value)} placeholder='{"name":"development","version":1,...}' /></label><button type="submit" disabled={busy || !definition.trim()}>Validate and publish</button></form></details>
    <div className="task-workflow-list"><h3>Explicit pools <span>{pools.length}</span></h3><ul>{pools.map((pool) => <li key={pool.name}><strong>{pool.name}</strong><span>{pool.agents.join(", ")} · rev {pool.revision}</span></li>)}</ul></div>
    <form onSubmit={(event) => { event.preventDefault(); void protect(async () => {
      const previous = pools.find((pool) => pool.name === poolName.trim())
      const updated = await rebindAgentPool(queue.prefix, poolName.trim(), poolAgents.split(",").map((agent) => agent.trim()).filter(Boolean), previous?.revision ?? 0, actionKey("pool"), target)
      setPools((current) => [...current.filter((pool) => pool.name !== updated.name), updated])
      toast.success("Agent pool updated")
    }) }}>
      <label>Pool name<Input aria-label={`Pool name ${queue.prefix}`} value={poolName} onChange={(event) => setPoolName(event.target.value)} /></label>
      <label>Explicit agents<Input aria-label={`Pool agents ${queue.prefix}`} value={poolAgents} onChange={(event) => setPoolAgents(event.target.value)} placeholder="dev-a, dev-b" /></label>
      <button type="submit" disabled={busy || !poolName.trim() || !poolAgents.trim()}>Save pool</button>
    </form>
    </fieldset>
  </section>
}
