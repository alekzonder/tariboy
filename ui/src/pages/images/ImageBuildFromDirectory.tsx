import { useState } from "react";
import { toast } from "sonner";
import {
  ApiError, buildImageDirectory, validateImageDirectory,
  type ImageDiagnostic, type ImageValidationResult,
} from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

const message = (error: unknown) => error instanceof ApiError ? error.message : String(error);

export default function ImageBuildFromDirectory() {
  const [path, setPath] = useState("");
  const [name, setName] = useState("");
  const [tag, setTag] = useState("latest");
  const [diagnostics, setDiagnostics] = useState<ImageDiagnostic[]>([]);
  const [validation, setValidation] = useState<ImageValidationResult | null>(null);
  const [busy, setBusy] = useState(false);
  const hasOutput = diagnostics.length > 0 || validation !== null;

  const closeOutput = () => {
    setDiagnostics([]);
    setValidation(null);
  };

  const validate = async () => {
    setBusy(true);
    try {
      const result = await validateImageDirectory({
        path: path.trim(), name: name.trim(),
        ...(tag.trim() && tag.trim() !== "latest" ? { tag: tag.trim() } : {}),
      });
      setValidation(result);
      setDiagnostics(result.diagnostics ?? []);
      if (result.valid) toast.success("Tariboyfile is valid");
    } catch (error) {
      toast.error(message(error));
    } finally {
      setBusy(false);
    }
  };

  const build = async () => {
    if (!name.trim()) {
      setDiagnostics([{ path: "name", message: "Image name is required" }]);
      return;
    }
    setBusy(true);
    try {
      const result = await buildImageDirectory({
        path: path.trim(), name: name.trim(),
        ...(tag.trim() && tag.trim() !== "latest" ? { tag: tag.trim() } : {}),
      });
      setDiagnostics([]);
      toast.success(`built ${result.name}:${result.tag}`);
      window.dispatchEvent(new Event("tariboy:image-built"));
    } catch (error) {
      toast.error(`build failed: ${message(error)}`);
    } finally {
      setBusy(false);
    }
  };

  return <section className="space-y-3 rounded border p-4">
    <div>
      <h2 className="font-medium">Build from directory</h2>
      <p className="text-sm text-muted-foreground">Uses the original directory containing Tariboyfile.yaml. Sources stay in place.</p>
    </div>
    <div className="grid gap-3 md:grid-cols-[1fr_14rem_10rem_auto_auto]">
      <Input aria-label="Image source directory" placeholder="/absolute/path/to/image" value={path} onChange={(event) => { setPath(event.target.value); setValidation(null); }} />
      <Input aria-label="Image name" required placeholder="name (required)" value={name} onChange={(event) => setName(event.target.value)} />
      <Input aria-label="Image tag" placeholder="latest" value={tag} onChange={(event) => setTag(event.target.value)} />
      <Button variant="outline" disabled={busy || !path.trim() || !name.trim()} onClick={() => void validate()}>Validate</Button>
      <Button disabled={busy || !path.trim() || !name.trim()} onClick={() => void build()}>Build</Button>
    </div>
    {hasOutput && <div className="flex justify-end">
      <Button type="button" variant="ghost" size="sm" onClick={closeOutput}>Close</Button>
    </div>}
    {diagnostics.map((item, index) => <p key={`${item.path}-${index}`} role="alert" className="text-sm text-destructive"><span className="font-mono">{item.path}</span>: {item.message}</p>)}
    {(validation?.warnings ?? []).map((item, index) => <p key={`warning-${item.path}-${index}`} className="text-sm text-amber-700"><span className="font-mono">{item.path}</span>: {item.message}</p>)}
    {validation?.valid && <div className="space-y-2 rounded border bg-muted/20 p-3" aria-label="Validated image template">
      <div className="text-xs text-muted-foreground">schema v{validation.schema_version} · template sha256 <span className="font-mono">{validation.template?.sha256 ?? "legacy"}</span></div>
      <div className="text-sm">Plugins: {(validation.plugins ?? []).length === 0 ? "none" : validation.plugins!.map((plugin) => <code key={plugin} className="ml-2">{plugin}</code>)}</div>
      <div className="space-y-1" aria-label="Validated image skills">
        <div className="text-sm">Skills: {(validation.skills ?? []).length === 0 ? "none" : null}</div>
        {(validation.skills ?? []).map((skill) => <div key={skill.name} className="rounded border bg-background p-2 text-sm">
          <strong>{skill.name}</strong>
          <p>{skill.description}</p>
          <div className="break-all font-mono text-xs">{skill.category} · {skill.source}</div>
          <div className="break-all text-xs text-muted-foreground">{skill.file_count} files · {skill.size} bytes · {skill.tree_sha256}</div>
        </div>)}
      </div>
      {validation.template && <ol className="space-y-1">
        {validation.template.entries.map((entry, index) => <li key={`${index}-${entry.archive_path ?? entry.runtime}`} className="rounded border bg-background p-2 text-sm">
          <span className="mr-3 font-mono text-muted-foreground">{index}</span>
          <strong className="mr-2">{entry.kind}</strong>
          {entry.runtime && <code>{entry.runtime}</code>}
          {entry.source && <div className="break-all font-mono text-xs">{entry.source}</div>}
          {entry.category && <div className="text-xs text-muted-foreground">{entry.category} · {entry.size ?? 0} bytes · {entry.sha256}</div>}
        </li>)}
      </ol>}
    </div>}
  </section>;
}
