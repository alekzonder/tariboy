import { useEffect, useState } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import {
  ApiError, createAgent, imageManifestGet, listGroups, listImages,
  type CreateAgentSpec, type GroupRow, type ImageManifest, type ImageRow,
} from "@/lib/api";
import { EditablePresetCombobox } from "@/components/EditablePresetCombobox";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { HARNESSES, ImageCombobox, serializeEnv, commaFieldError, type EnvRow } from "@/components/AgentFormFields";
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
import { rememberRuntimePreset, runtimePresetOptions } from "@/lib/runtimePresets";

export default function AgentCreate() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [images, setImages] = useState<ImageRow[]>([]);
  const [groups, setGroups] = useState<GroupRow[]>([]);
  const [manifest, setManifest] = useState<ImageManifest | null>(null);
  const [manifestImage, setManifestImage] = useState("");

  const [image, setImage] = useState(() => searchParams.get("image") ?? "");
  const [cwd, setCwd] = useState("");
  const [name, setName] = useState("");
  const [harness, setHarness] = useState("");
  const [model, setModel] = useState("");
  const [effort, setEffort] = useState("");
  const [loop, setLoop] = useState(true);
  const [interactive, setInteractive] = useState(false);
  const [envRows, setEnvRows] = useState<EnvRow[]>([]);
  const [plugins, setPlugins] = useState<string[]>([]);
  const [pluginDraft, setPluginDraft] = useState("");
  const [group, setGroup] = useState("");

  const [busy, setBusy] = useState(false);
  const [formErr, setFormErr] = useState<string | null>(null);
  const [fieldErr, setFieldErr] = useState<Record<string, string>>({});

  useEffect(() => {
    void listImages().then((r) => setImages(r.images ?? [])).catch(() => setImages([]));
    void listGroups().then((r) => setGroups(r.groups ?? [])).catch(() => setGroups([]));
  }, []);

  useEffect(() => {
    if (!image) return;
    let alive = true;
    void imageManifestGet(image)
      .then((next) => {
        if (!alive) return;
        setManifest(next);
        setManifestImage(image);
        if (next.schema_version === 1 && next.harness) {
          setHarness(next.harness.type ?? "");
          setModel(next.harness.model ?? "");
          setEffort(next.harness.effort ?? "");
        }
      })
      .catch(() => {
        if (alive) setManifest(null);
      });
    return () => {
      alive = false;
    };
  }, [image]);

  const currentManifest = manifestImage === image ? manifest : null;
  const allowsPluginOverrides = currentManifest?.schema_version === 1;
  const defaultHarness = allowsPluginOverrides ? currentManifest.harness?.type ?? "" : "";
  const defaultModel = allowsPluginOverrides ? currentManifest.harness?.model ?? "" : "";
  const defaultEffort = allowsPluginOverrides ? currentManifest.harness?.effort ?? "" : "";
  const modelPresets = runtimePresetOptions(harness, "models", [defaultModel, model]);
  const effortPresets = runtimePresetOptions(harness, "efforts", [defaultEffort, effort]);

  const addPlugin = () => {
    const p = pluginDraft.trim();
    if (p && !plugins.includes(p)) setPlugins([...plugins, p]);
    setPluginDraft("");
  };

  const changeImage = (value: string) => {
    setImage(value);
    setManifest(null);
    setManifestImage("");
  };

  const submit = async () => {
    setFormErr(null);
    setFieldErr({});
    const commaErr = commaFieldError(envRows, allowsPluginOverrides ? plugins : []);
    if (commaErr) {
      setFormErr(commaErr);
      return;
    }
    const spec: CreateAgentSpec = {
      image,
      name: name.trim() || undefined,
      cwd: cwd.trim() || undefined,
      harness: harness.trim()&&harness.trim()!==defaultHarness?harness.trim():undefined,
      model: model.trim()&&model.trim()!==defaultModel?model.trim():undefined,
      effort: effort.trim()&&effort.trim()!==defaultEffort?effort.trim():undefined,
      interactive,
      loop,
      env: serializeEnv(envRows) || undefined,
      plugins: allowsPluginOverrides ? plugins.join(",") || undefined : undefined,
      group: group || undefined,
    };
    setBusy(true);
    try {
      const r = await createAgent(spec);
      if (model.trim()&&model.trim()!==defaultModel) {
        rememberRuntimePreset(harness, "models", model);
      }
      if (effort.trim()&&effort.trim()!==defaultEffort) {
        rememberRuntimePreset(harness, "efforts", effort);
      }
      toast.success(`agent ${r.name} created`);
      navigate(`/agent/${encodeURIComponent(r.name)}`);
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : String(e);
      const code = e instanceof ApiError ? e.code : "";
      if (code === "bad_cwd") setFieldErr({ cwd: msg });
      else if (code === "missing_image") setFieldErr({ image: msg });
      else setFormErr(msg);
      toast.error(`create failed: ${msg}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4 p-6">
      <div className="flex items-center justify-between">
        <h1 className="text-lg font-semibold">New agent</h1>
        <Button variant="ghost" size="sm" onClick={() => navigate("/")}>Cancel</Button>
      </div>

      <Card>
        <CardHeader className="pb-2"><CardTitle className="text-base">Agent</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="image">image *</Label>
              <ImageCombobox
                id="image"
                images={images}
                value={image}
                onChange={changeImage}
                invalid={!!fieldErr.image}
              />
              {fieldErr.image && <p className="text-xs text-destructive">{fieldErr.image}</p>}
            </div>
            <div className="space-y-1">
              <Label htmlFor="cwd">cwd</Label>
              <PathAutocomplete
                id="cwd"
                value={cwd}
                onChange={setCwd}
                placeholder="empty = managed workdir"
                aria-label="cwd"
              />
              {fieldErr.cwd && <p className="text-xs text-destructive">{fieldErr.cwd}</p>}
            </div>
            <div className="space-y-1">
              <Label htmlFor="name">name</Label>
              <Input id="name" value={name} onChange={(e) => setName(e.target.value)}
                placeholder="empty = generated" className="h-8" />
            </div>
            <div className="space-y-1">
              <Label htmlFor="harness">harness</Label>
              <Select value={harness} onValueChange={setHarness}>
                <SelectTrigger id="harness" className="h-8"><SelectValue placeholder="CLI default" /></SelectTrigger>
                <SelectContent>
                  {HARNESSES.map((h) => <SelectItem key={h} value={h}>{h}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1">
              <Label htmlFor="model">model</Label>
              <EditablePresetCombobox
                id="model"
                ariaLabel="model"
                value={model}
                options={modelPresets}
                onChange={setModel}
                placeholder="image default"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="effort">effort</Label>
              <EditablePresetCombobox
                id="effort"
                ariaLabel="effort"
                value={effort}
                options={effortPresets}
                onChange={setEffort}
                placeholder="image default"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="group">group</Label>
              <Select value={group} onValueChange={setGroup}>
                <SelectTrigger id="group" className="h-8"><SelectValue placeholder="none" /></SelectTrigger>
                <SelectContent>
                  {groups.map((g) => <SelectItem key={g.name} value={g.name}>{g.name}</SelectItem>)}
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-end gap-2">
              <Switch id="loop" checked={loop} onCheckedChange={setLoop} />
              <Label htmlFor="loop">start loop</Label>
            </div>
          </div>

          <Collapsible>
            <CollapsibleTrigger asChild>
              <Button variant="outline" size="sm" type="button">Advanced ▾</Button>
            </CollapsibleTrigger>
            <CollapsibleContent className="mt-3 space-y-4">
              <div className="flex items-center gap-2">
                <Switch id="interactive" checked={interactive} onCheckedChange={setInteractive} />
                <Label htmlFor="interactive">interactive (tmux TUI)</Label>
              </div>

              <div className="space-y-2">
                <Label>env (K=V)</Label>
                {envRows.map((row, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <Input
                      aria-label={`env key ${i}`}
                      value={row.key}
                      onChange={(e) => setEnvRows(envRows.map((r, j) => j === i ? { ...r, key: e.target.value } : r))}
                      placeholder="KEY" className="h-8 w-40" />
                    <span className="text-muted-foreground">=</span>
                    <Input
                      aria-label={`env value ${i}`}
                      value={row.value}
                      onChange={(e) => setEnvRows(envRows.map((r, j) => j === i ? { ...r, value: e.target.value } : r))}
                      placeholder="value" className="h-8 w-48" />
                    <Button variant="ghost" size="sm" type="button"
                      onClick={() => setEnvRows(envRows.filter((_, j) => j !== i))}>Remove</Button>
                  </div>
                ))}
                <Button variant="outline" size="sm" type="button"
                  onClick={() => setEnvRows([...envRows, { key: "", value: "" }])}>+ Add env</Button>
              </div>

              {allowsPluginOverrides && <div className="space-y-2">
                <Label htmlFor="plugin-draft">plugins</Label>
                <div className="flex flex-wrap items-center gap-1">
                  {plugins.map((p) => (
                    <Badge key={p} variant="secondary" className="gap-1">
                      {p}
                      <button type="button" aria-label={`remove ${p}`}
                        onClick={() => setPlugins(plugins.filter((x) => x !== p))}>×</button>
                    </Badge>
                  ))}
                </div>
                <div className="flex items-center gap-2">
                  <Input
                    id="plugin-draft"
                    value={pluginDraft}
                    onChange={(e) => setPluginDraft(e.target.value)}
                    onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); addPlugin(); } }}
                    placeholder="plugin name" className="h-8 w-48" />
                  <Button variant="outline" size="sm" type="button" onClick={addPlugin}>Add plugin</Button>
                </div>
              </div>}
            </CollapsibleContent>
          </Collapsible>

          {formErr && <p className="text-sm text-destructive">{formErr}</p>}

          <div className="flex gap-2">
            <Button disabled={!image || busy || manifestImage !== image} onClick={() => void submit()}>
              {busy ? "Creating…" : "Create agent"}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
