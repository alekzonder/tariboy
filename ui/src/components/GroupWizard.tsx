import { useEffect, useState } from "react";
import {
  ApiError, apiOn, createAgent, getActiveDaemon, listImages,
  type CreateAgentSpec, type ImageRow,
} from "@/lib/api";
import { EFFORT_PRESETS } from "@/components/ComboField";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { ImageCombobox, serializeEnv, commaFieldError, type EnvRow } from "@/components/AgentFormFields";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";

// One draft agent row in the wizard. `id` is a stable key so per-row result
// status survives add/remove churn. `override` reveals the row's own cwd
// PathAutocomplete; when off, the row inherits the group's base cwd.
interface AgentDraft {
  id: number;
  image: string;
  name: string;
  override: boolean;
  cwd: string;
  model: string;
  effort: string;
  harness: string;
  timeout: string;
  interactive: boolean;
  loop: boolean;
  envRows: EnvRow[];
  plugins: string[];
  pluginDraft: string;
}

// Per-row orchestration outcome, keyed by draft id.
type RowStatus =
  | { kind: "creating" }
  | { kind: "created"; name: string }
  | { kind: "failed"; reason: string };

function emptyDraft(id: number): AgentDraft {
  return {
    id, image: "", name: "", override: false, cwd: "",
    model: "", effort: "", harness: "", timeout: "", interactive: false, loop: true, envRows: [], plugins: [], pluginDraft: "",
  };
}

// GroupWizard is the V4 multi-section group builder. Section 1 is the group
// (name + base cwd); the group lead is the row marked leader. Section 2 is a
// repeatable list of agents. "Create" is pure front-end orchestration: one
// POST /api/groups followed by one POST /api/agents per row (N+1 calls, in
// order). Partial failure is not rolled back — each row shows its own
// created/failed state and only failed rows are retried (the group already
// exists by then, so retry never re-creates it).
export function GroupWizard({ onCreated }: { onCreated?: () => void }) {
  const [images, setImages] = useState<ImageRow[]>([]);
  const [name, setName] = useState("");
  const [baseCwd, setBaseCwd] = useState("");
  const [leaderIdx, setLeaderIdx] = useState(0);
  const [nextId, setNextId] = useState(1);
  const [agents, setAgents] = useState<AgentDraft[]>([emptyDraft(0)]);
  const [target] = useState(() => getActiveDaemon());

  const [busy, setBusy] = useState(false);
  const [groupCreated, setGroupCreated] = useState(false);
  const [groupErr, setGroupErr] = useState<string | null>(null);
  const [formErr, setFormErr] = useState<string | null>(null);
  const [rowStatus, setRowStatus] = useState<Record<number, RowStatus>>({});

  useEffect(() => {
    void listImages(target).then((r) => setImages(r.images ?? [])).catch(() => setImages([]));
  }, [target]);

  const patch = (id: number, up: Partial<AgentDraft>) =>
    setAgents((rows) => rows.map((r) => (r.id === id ? { ...r, ...up } : r)));

  const addAgent = () => {
    setAgents((rows) => [...rows, emptyDraft(nextId)]);
    setNextId((n) => n + 1);
  };
  const removeAgent = (id: number, idx: number) => {
    setAgents((rows) => rows.filter((r) => r.id !== id));
    // Keep the leader marker pointing at a valid row.
    setLeaderIdx((li) => (idx < li ? li - 1 : idx === li ? 0 : li));
  };

  const leaderName = agents[leaderIdx]?.name.trim() || "";
  // Submit is gated on a group name, every row having an image, AND the leader
  // row having an explicit name. Without the last gate a blank leader name makes
  // lead=undefined at POST /api/groups, creating the group leaderless — the
  // intended leader then joins as a plain member via its group field.
  const canCreate =
    name.trim() !== "" && leaderName !== "" && agents.length > 0 && agents.every((a) => a.image);

  const imageSchemaVersion = (ref: string) => {
    const selected = images.find((candidate) => `${candidate.name}:${candidate.tag}` === ref);
    return selected?.schema_version;
  };

  const allowsPluginOverrides = (a: AgentDraft) => imageSchemaVersion(a.image) === 1;

  const specFor = (a: AgentDraft): CreateAgentSpec => ({
    image: a.image,
    name: a.name.trim() || undefined,
    cwd: (a.override ? a.cwd.trim() : baseCwd.trim()) || undefined,
    model: a.model.trim() || undefined,
    effort: a.effort || undefined,
    harness: a.harness.trim() || undefined,
    interactive: a.interactive,
    loop: a.loop,
    env: serializeEnv(a.envRows) || undefined,
    plugins: allowsPluginOverrides(a) ? a.plugins.join(",") || undefined : undefined,
    timeout: a.timeout.trim() || undefined,
    group: name.trim(),
  });

  // Run the N+1 orchestration over `drafts`. `createGroup` is false on retry
  // (the group already exists). Agents are created sequentially so the call
  // order is deterministic and the per-row status renders as each settles.
  const orchestrate = async (drafts: AgentDraft[], createGroup: boolean) => {
    setBusy(true);
    if (createGroup) {
      try {
        await apiOn(target, "POST", "/api/groups", { name: name.trim(), lead: leaderName || undefined });
        setGroupCreated(true);
        setGroupErr(null);
      } catch (e) {
        setGroupErr(e instanceof ApiError ? e.message : String(e));
        setBusy(false);
        return;
      }
    }
    for (const a of drafts) {
      setRowStatus((s) => ({ ...s, [a.id]: { kind: "creating" } }));
      try {
        const r = await createAgent(specFor(a), target);
        setRowStatus((s) => ({ ...s, [a.id]: { kind: "created", name: r.name } }));
      } catch (e) {
        const reason = e instanceof ApiError ? e.message : String(e);
        setRowStatus((s) => ({ ...s, [a.id]: { kind: "failed", reason } }));
      }
    }
    setBusy(false);
    onCreated?.();
  };

  const create = () => {
    setFormErr(null);
    for (const a of agents) {
      const commaErr = commaFieldError(a.envRows, allowsPluginOverrides(a) ? a.plugins : []);
      if (commaErr) {
        setFormErr(`${a.name.trim() || "agent"}: ${commaErr}`);
        return;
      }
    }
    void orchestrate(agents, true);
  };
  const retryFailed = () => {
    const failed = agents.filter((a) => rowStatus[a.id]?.kind === "failed");
    if (failed.length) void orchestrate(failed, false);
  };

  const anyFailed = agents.some((a) => rowStatus[a.id]?.kind === "failed");

  return (
    <Card>
      <CardHeader className="pb-2"><CardTitle className="text-base">New group (wizard)</CardTitle></CardHeader>
      <CardContent className="space-y-4">
        {/* Section 1 — group */}
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1">
            <Label htmlFor="grp-name">group name *</Label>
            <Input id="grp-name" value={name} onChange={(e) => setName(e.target.value)}
              placeholder="group name" className="h-8" />
          </div>
          <div className="space-y-1">
            <Label htmlFor="grp-cwd">base cwd</Label>
            <PathAutocomplete
              id="grp-cwd"
              value={baseCwd}
              onChange={setBaseCwd}
              placeholder="default for every agent"
              aria-label="base cwd"
            />
          </div>
        </div>
        <p className="text-xs text-muted-foreground">
          lead: <span className="font-mono">{leaderName || "—"}</span> (the agent marked leader below)
        </p>
        {leaderName === "" && (
          <p className="text-xs text-destructive">
            the leader row needs an explicit name so the group is created with a lead
          </p>
        )}

        {/* Section 2 — agents */}
        <div className="space-y-3">
          {agents.map((a, i) => {
            const st = rowStatus[a.id];
            return (
              <div key={a.id} data-slot="agent-row" className="space-y-3 rounded-lg border p-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2 text-sm">
                    <label className="flex items-center gap-1.5">
                      <input
                        type="radio"
                        name="leader"
                        aria-label={`leader ${i}`}
                        checked={leaderIdx === i}
                        onChange={() => setLeaderIdx(i)}
                      />
                      <span className="text-muted-foreground">{leaderIdx === i ? "lead" : "worker"}</span>
                    </label>
                    {st?.kind === "creating" && <Badge variant="secondary">creating…</Badge>}
                    {st?.kind === "created" && <Badge>created {st.name}</Badge>}
                    {st?.kind === "failed" && (
                      <Badge variant="destructive" title={st.reason}>failed: {st.reason}</Badge>
                    )}
                  </div>
                  {agents.length > 1 && (
                    <Button variant="ghost" size="sm" type="button"
                      onClick={() => removeAgent(a.id, i)}>Remove agent</Button>
                  )}
                </div>

                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-1">
                    <Label htmlFor={`img-${a.id}`}>image *</Label>
                    <ImageCombobox
                      id={`img-${a.id}`}
                      ariaLabel={`image ${i}`}
                      images={images}
                      value={a.image}
                      onChange={(v) => patch(a.id, { image: v })}
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor={`name-${a.id}`}>name</Label>
                    <Input id={`name-${a.id}`} value={a.name}
                      onChange={(e) => patch(a.id, { name: e.target.value })}
                      placeholder="empty = generated" className="h-8" />
                  </div>
                </div>

                <div className="flex items-center gap-2">
                  <Switch id={`override-${a.id}`} checked={a.override}
                    onCheckedChange={(v) => patch(a.id, { override: v })} />
                  <Label htmlFor={`override-${a.id}`}>override cwd</Label>
                </div>
                {a.override && (
                  <PathAutocomplete
                    value={a.cwd}
                    onChange={(v) => patch(a.id, { cwd: v })}
                    placeholder="this agent's cwd"
                    aria-label={`cwd ${i}`}
                  />
                )}

                <Collapsible>
                  <CollapsibleTrigger asChild>
                    <Button variant="outline" size="sm" type="button">Advanced ▾</Button>
                  </CollapsibleTrigger>
                  <CollapsibleContent className="mt-3 space-y-4">
                    <div className="grid gap-4 sm:grid-cols-2">
                      <div className="space-y-1">
                        <Label htmlFor={`harness-${a.id}`}>harness</Label>
                        <Input id={`harness-${a.id}`} value={a.harness}
                          onChange={(e) => patch(a.id, { harness: e.target.value })}
                          placeholder="image default" className="h-8" />
                      </div>
                      <div className="space-y-1">
                        <Label htmlFor={`model-${a.id}`}>model</Label>
                        <Input id={`model-${a.id}`} value={a.model}
                          onChange={(e) => patch(a.id, { model: e.target.value })}
                          placeholder="harness default" className="h-8" />
                      </div>
                      <div className="space-y-1">
                        <Label htmlFor={`timeout-${a.id}`}>timeout</Label>
                        <Input id={`timeout-${a.id}`} aria-label={`timeout ${i}`} value={a.timeout}
                          onChange={(e) => patch(a.id, { timeout: e.target.value })}
                          placeholder="for example 60m or 2h" className="h-8" />
                      </div>
                      <div className="space-y-1">
                        <Label htmlFor={`effort-${a.id}`}>effort</Label>
                        <Select value={a.effort} onValueChange={(v) => patch(a.id, { effort: v })}>
                          <SelectTrigger id={`effort-${a.id}`} className="h-8"><SelectValue placeholder="default" /></SelectTrigger>
                          <SelectContent>
                            {EFFORT_PRESETS.map((p) => <SelectItem key={p} value={p}>{p}</SelectItem>)}
                          </SelectContent>
                        </Select>
                      </div>
                    </div>

                    <div className="flex items-center gap-2">
                      <Switch id={`interactive-${a.id}`} checked={a.interactive}
                        onCheckedChange={(v) => patch(a.id, { interactive: v })} />
                      <Label htmlFor={`interactive-${a.id}`}>interactive (tmux TUI)</Label>
                    </div>
                    <div className="flex items-center gap-2">
                      <Switch id={`loop-${a.id}`} checked={a.loop}
                        onCheckedChange={(v) => patch(a.id, { loop: v })} />
                      <Label htmlFor={`loop-${a.id}`} aria-label={`autopilot ${i}`}>Autopilot</Label>
                    </div>

                    <div className="space-y-2">
                      <Label>env (K=V)</Label>
                      {a.envRows.map((row, j) => (
                        <div key={j} className="flex items-center gap-2">
                          <Input
                            aria-label={`env key ${i}.${j}`}
                            value={row.key}
                            onChange={(e) => patch(a.id, { envRows: a.envRows.map((r, k) => k === j ? { ...r, key: e.target.value } : r) })}
                            placeholder="KEY" className="h-8 w-40" />
                          <span className="text-muted-foreground">=</span>
                          <Input
                            aria-label={`env value ${i}.${j}`}
                            value={row.value}
                            onChange={(e) => patch(a.id, { envRows: a.envRows.map((r, k) => k === j ? { ...r, value: e.target.value } : r) })}
                            placeholder="value" className="h-8 w-48" />
                          <Button variant="ghost" size="sm" type="button"
                            onClick={() => patch(a.id, { envRows: a.envRows.filter((_, k) => k !== j) })}>Remove</Button>
                        </div>
                      ))}
                      <Button variant="outline" size="sm" type="button"
                        onClick={() => patch(a.id, { envRows: [...a.envRows, { key: "", value: "" }] })}>+ Add env</Button>
                    </div>

                    {allowsPluginOverrides(a) && <div className="space-y-2">
                      <Label htmlFor={`plugin-${a.id}`}>plugins</Label>
                      <div className="flex flex-wrap items-center gap-1">
                        {a.plugins.map((p) => (
                          <Badge key={p} variant="secondary" className="gap-1">
                            {p}
                            <button type="button" aria-label={`remove ${p}`}
                              onClick={() => patch(a.id, { plugins: a.plugins.filter((x) => x !== p) })}>×</button>
                          </Badge>
                        ))}
                      </div>
                      <div className="flex items-center gap-2">
                        <Input
                          id={`plugin-${a.id}`}
                          value={a.pluginDraft}
                          onChange={(e) => patch(a.id, { pluginDraft: e.target.value })}
                          onKeyDown={(e) => {
                            if (e.key === "Enter") {
                              e.preventDefault();
                              const p = a.pluginDraft.trim();
                              patch(a.id, {
                                plugins: p && !a.plugins.includes(p) ? [...a.plugins, p] : a.plugins,
                                pluginDraft: "",
                              });
                            }
                          }}
                          placeholder="plugin name" className="h-8 w-48" />
                        <Button variant="outline" size="sm" type="button" onClick={() => {
                          const p = a.pluginDraft.trim();
                          patch(a.id, {
                            plugins: p && !a.plugins.includes(p) ? [...a.plugins, p] : a.plugins,
                            pluginDraft: "",
                          });
                        }}>Add plugin</Button>
                      </div>
                    </div>}
                  </CollapsibleContent>
                </Collapsible>
              </div>
            );
          })}
          <Button variant="outline" size="sm" type="button" onClick={addAgent}>+ Add agent</Button>
        </div>

        {formErr && <p className="text-sm text-destructive">{formErr}</p>}
        {groupErr && <p className="text-sm text-destructive">group create failed: {groupErr}</p>}

        <div className="flex gap-2">
          <Button disabled={!canCreate || busy} onClick={create}>
            {busy ? "Creating…" : "Create group"}
          </Button>
          {anyFailed && !busy && (
            <Button variant="outline" onClick={retryFailed}>Retry failed</Button>
          )}
        </div>
        {groupCreated && (
          <p className="text-xs text-muted-foreground">
            group <span className="font-mono">{name.trim()}</span> created; agents joined via their group field.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
