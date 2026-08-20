import { useEffect, useState } from "react";
import { apiOn, getActiveDaemon } from "@/lib/api";
import { guard } from "@/lib/toast-guard";
import { GroupWizard } from "@/components/GroupWizard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { toast } from "sonner";
import { useSearchParams } from "react-router-dom";
import {
  applyTeamArchiveOn, downloadTeamArchiveOn, getTeamComposeOn, importTeamYAMLOn,
  getTeamImportOperationOn, removeTeamMemberOn, renameTeamOn, setTeamLeadOn, uploadTeamArchiveOn,
  type TeamImportOperation,
} from "@/lib/teamApi";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

interface GroupRow { name: string; lead: string; members: number }
interface GroupView { name: string; lead: string; members: string[]; broadcast: string; inbox: string; shared_dir: string }

export default function GroupsPage() {
  const [searchParams] = useSearchParams();
  const [target] = useState(() => getActiveDaemon());
  const [groups, setGroups] = useState<GroupRow[]>([]);
  const [sel, setSel] = useState<GroupView | null>(null);
  const [newName, setNewName] = useState("");
  const [newLead, setNewLead] = useState("");
  const [assignAgent, setAssignAgent] = useState("");
  const [wizard, setWizard] = useState(false);
  const [rename, setRename] = useState("");
  const [yaml, setYaml] = useState("");
  const [archivePreview, setArchivePreview] = useState<null | { import_id: string; team: string; yaml: string; images: Array<{original: string; resolved: string; action: string; conflict: boolean; message?: string}>; agents: Array<{name: string; action: string; conflict: boolean; message?: string}>; groups: Array<{name: string; action: string; conflict: boolean; message?: string}>; updateExisting: boolean; target: ReturnType<typeof getActiveDaemon> }>(null);
  const [importOperation, setImportOperation] = useState<TeamImportOperation | null>(null);
  const [operationRef, setOperationRef] = useState<{ id: string; target: ReturnType<typeof getActiveDaemon> } | null>(() => {
    const saved = localStorage.getItem("tariboy:team-import");
    if (!saved) return null;
    try {
      const record = JSON.parse(saved) as { id: string; hostId: string };
      return record.hostId === (target?.id ?? "") ? { id: record.id, target } : null;
    } catch {
      localStorage.removeItem("tariboy:team-import");
      return null;
    }
  });

  const refreshSelected = (team = sel?.name) => {
    if (team) void inspect(team);
    void load();
  };

  const saveBlob = (blob: Blob, filename: string) => {
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url; link.download = filename; link.click();
    URL.revokeObjectURL(url);
  };

  const load = () => apiOn<{ groups: GroupRow[] }>(target, "GET", "/api/groups").then((r) => setGroups(r.groups)).catch(() => setGroups([]));
  const inspect = (name: string) => apiOn<GroupView>(target, "GET", `/api/groups/${encodeURIComponent(name)}`).then((group) => { setSel(group); setRename(group.name); }).catch(() => setSel(null));
  useEffect(() => { void load(); const requested = searchParams.get("team"); if (requested) void inspect(requested); }, [target]);
  useEffect(() => {
    if (!operationRef) return;
    void getTeamImportOperationOn(operationRef.target, operationRef.id).then(setImportOperation).catch(() => localStorage.removeItem("tariboy:team-import"));
  }, [operationRef]);
  useEffect(() => {
    if (!operationRef || importOperation?.status !== "running") return;
    const timer = window.setInterval(() => void getTeamImportOperationOn(operationRef.target, operationRef.id).then((operation) => {
      setImportOperation(operation);
      if (operation.status === "complete") { localStorage.removeItem("tariboy:team-import"); void load(); }
    }).catch(() => {}), 500);
    return () => window.clearInterval(timer);
  }, [operationRef, importOperation?.status]);

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">Groups</h1>
        <Button size="sm" variant={wizard ? "secondary" : "default"} onClick={() => setWizard((w) => !w)}>
          {wizard ? "Close wizard" : "New group (wizard)"}
        </Button>
      </div>

      {wizard ? (
        <GroupWizard onCreated={() => void load()} />
      ) : (
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-base">Create group</CardTitle></CardHeader>
          <CardContent className="flex flex-wrap items-end gap-2">
            <Input value={newName} onChange={(e) => setNewName(e.target.value)} placeholder="group name" className="h-8 w-48" />
            <Input value={newLead} onChange={(e) => setNewLead(e.target.value)} placeholder="lead (optional)" className="h-8 w-48" />
            <Button size="sm" onClick={() => guard("create", () => apiOn(target, "POST", "/api/groups", { name: newName, lead: newLead || undefined }), { verb: "ok", after: () => { setNewName(""); setNewLead(""); void load(); } })}>Create</Button>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader className="pb-2"><CardTitle className="text-base">Groups</CardTitle></CardHeader>
          <CardContent className="space-y-1">
            {groups.map((g) => (
              <button key={g.name} onClick={() => void inspect(g.name)} className="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-sm hover:bg-accent">
                <span className="font-mono">{g.name}</span>
                <span className="flex items-center gap-2 text-xs text-muted-foreground">
                  {g.lead && <Badge variant="secondary">lead {g.lead}</Badge>}
                  <span>{g.members} members</span>
                </span>
              </button>
            ))}
            {groups.length === 0 && <p className="text-sm text-muted-foreground">No groups.</p>}
          </CardContent>
        </Card>

        {sel && (
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-base font-mono">{sel.name}</CardTitle>
              <AlertDialog>
                <AlertDialogTrigger asChild><Button size="sm" variant="destructive">Remove</Button></AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>Remove group {sel.name}?</AlertDialogTitle>
                    <AlertDialogDescription>Detaches members and deletes the group's channels.</AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction onClick={() => guard("remove", () => apiOn(target, "DELETE", `/api/groups/${encodeURIComponent(sel.name)}`), { verb: "ok", after: () => { setSel(null); void load(); } })}>Remove</AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              <div className="flex gap-2">
                <Input aria-label="team name" value={rename} onChange={(event) => setRename(event.target.value)} className="h-8" />
                <Button size="sm" variant="outline" onClick={() => guard("rename", () => renameTeamOn(target, sel.name, rename), { verb: "ok", after: () => { setSel(null); void inspect(rename); void load(); } })}>Rename</Button>
              </div>
              <div className="flex items-center gap-2">lead:
                <select aria-label="team lead" value={sel.lead} className="h-8 rounded border bg-background px-2"
                  onChange={(event) => guard("lead", () => setTeamLeadOn(target, sel.name, event.target.value), { verb: "ok", after: () => refreshSelected() })}>
                  <option value="">No lead</option>
                  {sel.members.map((member) => <option key={member} value={member}>{member}</option>)}
                </select>
              </div>
              <div className="space-y-1">members:
                {sel.members.map((member) => (
                  <div key={member} className="flex items-center justify-between rounded border px-2 py-1">
                    <span className="font-mono">{member}</span>
                    <Button size="sm" variant="ghost" aria-label={`Remove ${member}`}
                      onClick={() => guard("remove member", () => removeTeamMemberOn(target, sel.name, member), { verb: "ok", after: () => refreshSelected() })}>Remove</Button>
                  </div>
                ))}
              </div>
              <div className="text-xs text-muted-foreground">shared: {sel.shared_dir}</div>
              <div className="flex items-end gap-2 pt-2">
                <Input value={assignAgent} onChange={(e) => setAssignAgent(e.target.value)} placeholder="agent to assign" className="h-8 w-48" />
                <Button size="sm" onClick={() => guard("assign", () => apiOn(target, "POST", `/api/groups/${encodeURIComponent(sel.name)}/assign`, { agent: assignAgent }), { verb: "ok", after: () => { setAssignAgent(""); void inspect(sel.name); void load(); } })}>Assign</Button>
              </div>
              <div className="flex flex-wrap gap-2 border-t pt-3">
                <Button size="sm" variant="outline" aria-label="Copy compose YAML" onClick={() => void getTeamComposeOn(target, sel.name).then((result) => navigator.clipboard.writeText(result.yaml)).then(() => toast.success("compose YAML copied")).catch((error) => toast.error(String(error)))}>Copy YAML</Button>
                <Button size="sm" variant="outline" aria-label="Export team archive" onClick={() => void downloadTeamArchiveOn(target, sel.name).then((blob) => saveBlob(blob, `${sel.name}.tar.gz`)).catch((error) => toast.error(String(error)))}>Export archive</Button>
              </div>
            </CardContent>
          </Card>
        )}
      </div>
      <Card>
        <CardHeader className="pb-2"><CardTitle className="text-base">Import team</CardTitle></CardHeader>
        <CardContent className="space-y-2">
          <Textarea aria-label="Import compose YAML" value={yaml} onChange={(event) => setYaml(event.target.value)} placeholder="Paste tariboy-compose.yaml" className="min-h-32 w-full rounded border bg-background p-2 font-mono text-xs" />
          <Button size="sm" disabled={!yaml.trim()} onClick={() => void importTeamYAMLOn(target, yaml).then((preview) => {
            setArchivePreview({ ...preview, images: preview.images.map((image) => ({ ...image, original: image.ref, resolved: image.ref })), groups: preview.groups ?? [], updateExisting: false, target });
            setOperationRef({ id: preview.import_id, target });
            setImportOperation({ import_id: preview.import_id, team: preview.team, status: "pending", steps: [], updated_at: "" });
            setYaml("");
          }).catch((error) => toast.error(String(error)))}>Preview YAML</Button>
          <div>
            <label className="text-sm font-medium" htmlFor="team-archive">Portable team archive (compose only; transfer images separately)</label>
            <Input id="team-archive" aria-label="Import team archive" type="file" accept=".gz,.tgz,application/gzip" className="mt-1"
              onChange={(event) => { const file = event.target.files?.[0]; if (!file) return; const target = getActiveDaemon(); void uploadTeamArchiveOn(target, file).then((preview) => { setArchivePreview({ ...preview, agents: preview.agents ?? [], groups: preview.groups ?? [], images: preview.images.map((image) => ({ ...image, original: image.ref, resolved: image.ref })), updateExisting: false, target }); setOperationRef({ id: preview.import_id, target }); setImportOperation({ import_id: preview.import_id, team: preview.team, status: "pending", steps: preview.images.map((image) => ({ kind: "image", name: image.ref, status: "pending" })), updated_at: "" }); }).catch((error) => toast.error(String(error))); }} />
          </div>
          {archivePreview && <div className="space-y-2 rounded border p-3 text-sm">
            <div>Team: <strong>{archivePreview.team}</strong></div>
            <div className="space-y-1">Images: {archivePreview.images.length === 0 && "none"}
              {archivePreview.images.map((image, index) => <div key={image.original}><Input aria-label={`Imported team image ref ${index}`} value={image.resolved} className="h-8 font-mono"
                onChange={(event) => setArchivePreview({ ...archivePreview, images: archivePreview.images.map((candidate, candidateIndex) => candidateIndex === index ? { ...candidate, resolved: event.target.value } : candidate) })} />
                <span className={image.conflict ? "text-destructive" : "text-muted-foreground"}>{image.action}{image.message ? ` — ${image.message}` : ""}</span></div>)}
            </div>
            {archivePreview.agents.map((agent) => <div key={agent.name} className={agent.conflict ? "text-destructive" : "text-muted-foreground"}>agent {agent.name}: {agent.action}{agent.message ? ` — ${agent.message}; edit YAML to rename it` : ""}</div>)}
            {archivePreview.groups.map((group) => <div key={group.name} className={group.conflict ? "text-destructive" : "text-muted-foreground"}>team {group.name}: {group.action}{group.message ? ` — ${group.message}` : ""}</div>)}
            {archivePreview.groups.some((group) => group.conflict) && <label className="flex items-center gap-2"><input aria-label="Update existing team" type="checkbox" checked={archivePreview.updateExisting} onChange={(event) => setArchivePreview({ ...archivePreview, updateExisting: event.target.checked })} />Update the existing team additively (members not present in YAML are preserved)</label>}
            <Textarea aria-label="Imported team compose YAML" value={archivePreview.yaml} onChange={(event) => setArchivePreview({ ...archivePreview, yaml: event.target.value })} className="max-h-48 min-h-32 w-full overflow-auto rounded bg-muted p-2 font-mono text-xs" />
            <Button size="sm" aria-label="Confirm and import team" onClick={() => {
              const preview = archivePreview;
              const refs = Object.fromEntries(preview.images.filter((image) => image.original !== image.resolved).map((image) => [image.original, image.resolved]));
              localStorage.setItem("tariboy:team-import", JSON.stringify({ id: preview.import_id, hostId: preview.target?.id ?? "" }));
              setImportOperation((operation) => operation ? { ...operation, status: "running" } : operation);
              void applyTeamArchiveOn(preview.target, preview.import_id, { refs: Object.keys(refs).length ? refs : undefined, yaml: preview.yaml, update_existing: preview.updateExisting })
                .then(() => getTeamImportOperationOn(preview.target, preview.import_id))
                .then((operation) => { setImportOperation(operation); if (operation.status === "complete") { localStorage.removeItem("tariboy:team-import"); toast.success("team archive imported"); setArchivePreview(null); void load(); } })
                .catch(async (error) => { try { setImportOperation(await getTeamImportOperationOn(preview.target, preview.import_id)); } catch { /* preserve apply error */ } toast.error(String(error)); });
            }}>Confirm and import team</Button>
          </div>}
          {importOperation && <div className="space-y-1 text-xs" aria-label="Team import progress">
            <div>Import status: {importOperation.status}</div>
            {importOperation.steps.map((step, index) => <div key={`${step.kind}-${step.name}-${index}`}>{step.kind} {step.name}: {step.status}{step.error ? ` — ${step.error}` : ""}</div>)}
            {importOperation.status === "failed" && operationRef && <Button size="sm" variant="outline" onClick={() => { setImportOperation({ ...importOperation, status: "running" }); void applyTeamArchiveOn(operationRef.target, operationRef.id).then(() => getTeamImportOperationOn(operationRef.target, operationRef.id)).then(setImportOperation).catch(() => void getTeamImportOperationOn(operationRef.target, operationRef.id).then(setImportOperation)); }}>Retry unfinished work</Button>}
          </div>}
        </CardContent>
      </Card>
    </div>
  );
}
