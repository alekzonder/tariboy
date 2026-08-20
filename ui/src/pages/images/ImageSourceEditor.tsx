import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import CodeMirror from "@uiw/react-codemirror";
import { yaml } from "@codemirror/lang-yaml";
import { markdown } from "@codemirror/lang-markdown";
import type { Extension } from "@codemirror/state";
import { ApiError } from "@/lib/api";
import { useOptionalDaemons } from "@/components/DaemonProvider";
import {
  buildImageSource,
  getImageSource,
  getImageSourceFile,
  listImageSourceFiles,
  putImageSourceFile,
  validateImageSource,
  type ImageBuildResult,
  type ImageDiagnostic,
  type ImageSource,
  type ImageSourceFile,
} from "@/lib/imageSources";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

function message(error: unknown): string {
  return error instanceof ApiError ? error.message : String(error);
}

function language(path: string): Extension[] {
  if (path.endsWith(".yaml") || path.endsWith(".yml")) return [yaml()];
  if (path.endsWith(".md") || path.endsWith(".markdown")) return [markdown()];
  return [];
}

export default function ImageSourceEditor() {
  const { name = "" } = useParams();
  const hostKey = useOptionalDaemons()?.activeId ?? "";
  const [source, setSource] = useState<ImageSource | null>(null);
  const [files, setFiles] = useState<ImageSourceFile[]>([]);
  const [selected, setSelected] = useState("");
  const [saved, setSaved] = useState("");
  const [draft, setDraft] = useState("");
  const [tag, setTag] = useState("latest");
  const [diagnostics, setDiagnostics] = useState<ImageDiagnostic[]>([]);
  const [validationOK, setValidationOK] = useState(false);
  const [buildResult, setBuildResult] = useState<ImageBuildResult | null>(null);
  const [busy, setBusy] = useState<"" | "save" | "validate" | "build">("");
  const [error, setError] = useState("");
  const dirty = selected !== "" && draft !== saved;

  useEffect(() => {
    let alive = true;
    void Promise.all([getImageSource(name), listImageSourceFiles(name)])
      .then(([nextSource, listing]) => {
        if (!alive) return;
        const editable = (listing.files ?? []).filter(
          (file) => file.path !== ".tariboy-source.json",
        );
        setSource(nextSource);
        setFiles(editable);
        setSelected(
          editable.find((file) => file.path === "Tariboyfile.yaml")?.path
            ?? editable[0]?.path
            ?? "",
        );
      })
      .catch((err) => {
        if (alive) setError(message(err));
      });
    return () => { alive = false; };
  }, [hostKey, name]);

  useEffect(() => {
    if (!selected) return;
    let alive = true;
    void getImageSourceFile(name, selected)
      .then((file) => {
        if (alive) {
          setSaved(file.content);
          setDraft(file.content);
          setError("");
        }
      })
      .catch((err) => {
        if (alive) setError(message(err));
      });
    return () => { alive = false; };
  }, [hostKey, name, selected]);

  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    const guardLink = (event: MouseEvent) => {
      const target = event.target;
      if (!(target instanceof Element) || !target.closest("a[href]")) return;
      if (window.confirm("Discard unsaved changes?")) return;
      event.preventDefault();
      event.stopPropagation();
    };
    window.addEventListener("beforeunload", warn);
    document.addEventListener("click", guardLink, true);
    return () => {
      window.removeEventListener("beforeunload", warn);
      document.removeEventListener("click", guardLink, true);
    };
  }, [dirty]);

  const extensions = useMemo(() => language(selected), [selected]);

  const choose = (path: string) => {
    if (path === selected) return;
    if (dirty && !window.confirm("Discard unsaved changes?")) return;
    setSelected(path);
    setDiagnostics([]);
    setValidationOK(false);
  };

  const save = async () => {
    if (!selected) return;
    setBusy("save");
    setError("");
    try {
      await putImageSourceFile(name, selected, draft);
      setSaved(draft);
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy("");
    }
  };

  const runValidation = async () => {
    setBusy("validate");
    setError("");
    setBuildResult(null);
    try {
      const result = await validateImageSource(name);
      setDiagnostics(result.diagnostics ?? []);
      setValidationOK(result.valid);
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy("");
    }
  };

  const runBuild = async () => {
    setBusy("build");
    setError("");
    setBuildResult(null);
    try {
      const result = await buildImageSource(name, tag || "latest");
      setBuildResult(result);
      setDiagnostics([]);
      setValidationOK(true);
      window.dispatchEvent(new CustomEvent("tariboy:image-built", { detail: result }));
      setSource((current) => current ? {
        ...current,
        last_build: {
          ref: result.ref,
          digest: result.digest,
          built_at: result.built_at,
        },
      } : current);
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy("");
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="flex flex-wrap items-center gap-2 border-b px-4 py-2">
        <Button size="sm" variant="ghost" asChild><Link to="/images">← Images</Link></Button>
        <h1 className="font-mono text-sm font-semibold">{name}</h1>
        {dirty && <span className="text-xs text-amber-600">Unsaved changes</span>}
        <div className="ml-auto flex items-center gap-2">
          <Label htmlFor="image-tag" className="text-xs">tag</Label>
          <Input
            id="image-tag"
            value={tag}
            onChange={(event) => setTag(event.target.value)}
            className="h-8 w-28"
          />
          <Button size="sm" onClick={() => void save()} disabled={!dirty || busy !== ""}>
            {busy === "save" ? "Saving…" : "Save"}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => void runValidation()}
            disabled={dirty || busy !== ""}
          >
            {busy === "validate" ? "Validating…" : "Validate"}
          </Button>
          <Button
            size="sm"
            onClick={() => void runBuild()}
            disabled={dirty || busy !== "" || !tag.trim()}
          >
            {busy === "build" ? "Building…" : "Build"}
          </Button>
        </div>
      </header>
      {(error || validationOK || diagnostics.length > 0 || buildResult) && (
        <div className="space-y-2 border-b px-4 py-2 text-sm">
          {error && <p role="alert" className="text-destructive">{error}</p>}
          {validationOK && !buildResult && <p className="text-emerald-600">Source is valid.</p>}
          {diagnostics.map((diagnostic, index) => (
            <button
              key={`${diagnostic.path}-${diagnostic.field ?? ""}-${index}`}
              type="button"
              className="block text-left text-destructive hover:underline"
              onClick={() => choose(diagnostic.path)}
            >
              {diagnostic.path}
              {diagnostic.field ? ` · ${diagnostic.field}` : ""}: {diagnostic.message}
            </button>
          ))}
          {buildResult && (
            <div className="flex flex-wrap items-center gap-3 text-emerald-600">
              <span>Built</span>
              <span className="font-mono">{buildResult.ref}</span>
              <span className="font-mono text-xs">{buildResult.digest}</span>
              <Link className="underline" to="/images?tab=built">View Built</Link>
            </div>
          )}
        </div>
      )}
      <div className="grid min-h-0 flex-1 grid-cols-[240px_minmax(0,1fr)]">
        <aside className="overflow-auto border-r p-2">
          <div className="mb-2 px-2 text-xs font-medium text-muted-foreground">Source files</div>
          {files.map((file) => (
            <button
              key={file.path}
              type="button"
              onClick={() => choose(file.path)}
              className={`block w-full rounded px-2 py-1.5 text-left font-mono text-xs ${
                selected === file.path ? "bg-accent" : "hover:bg-accent/60"
              }`}
            >
              {file.path}
            </button>
          ))}
        </aside>
        <main className="min-h-0 overflow-auto">
          {selected ? (
            <CodeMirror
              aria-label="source editor"
              value={draft}
              extensions={extensions}
              onChange={setDraft}
              height="100%"
            />
          ) : (
            <div className="p-6 text-sm text-muted-foreground">No editable files.</div>
          )}
        </main>
      </div>
      {source?.last_build && (
        <footer className="border-t px-4 py-2 text-xs text-muted-foreground">
          Last successful build: <span className="font-mono">{source.last_build.ref}</span>
          {" · "}
          <span className="font-mono">{source.last_build.digest.slice(0, 18)}</span>
        </footer>
      )}
    </div>
  );
}
