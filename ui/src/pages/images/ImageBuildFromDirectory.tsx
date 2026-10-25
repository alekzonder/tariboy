import { useState } from "react";
import { toast } from "sonner";
import { type ImageDiagnostic, type ImageValidationResult } from "@/lib/api";
import { notifyImagesChanged } from "@/lib/imageEvents";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { errorText } from "@/lib/errors";
import { directoryRequest } from "./directoryRequest";
import { ValidatedTemplate } from "./ValidatedTemplate";

export default function ImageBuildFromDirectory() {
  const api = useApi();
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
      const result = await api.images.validateImageDirectory(directoryRequest(path, name, tag));
      setValidation(result);
      setDiagnostics(result.diagnostics ?? []);
      if (result.valid) toast.success("Tariboyfile is valid");
    } catch (error) {
      toast.error(errorText(error));
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
      const result = await api.images.buildImageDirectory(directoryRequest(path, name, tag));
      setDiagnostics([]);
      toast.success(`built ${result.name}:${result.tag}`);
      notifyImagesChanged();
    } catch (error) {
      toast.error(`build failed: ${errorText(error)}`);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="space-y-3 rounded border p-4">
      <div>
        <h2 className="font-medium">Build from directory</h2>
        <p className="text-sm text-muted-foreground">
          Uses the original directory containing Tariboyfile.yaml. Sources stay in place.
        </p>
      </div>
      <div className="grid gap-3 md:grid-cols-[1fr_14rem_10rem_auto_auto]">
        <Input
          aria-label="Image source directory"
          placeholder="/absolute/path/to/image"
          value={path}
          onChange={(event) => {
            setPath(event.target.value);
            setValidation(null);
          }}
        />
        <Input
          aria-label="Image name"
          required
          placeholder="name (required)"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
        <Input
          aria-label="Image tag"
          placeholder="latest"
          value={tag}
          onChange={(event) => setTag(event.target.value)}
        />
        <Button
          variant="outline"
          disabled={busy || !path.trim() || !name.trim()}
          onClick={() => void validate()}
        >
          Validate
        </Button>
        <Button disabled={busy || !path.trim() || !name.trim()} onClick={() => void build()}>
          Build
        </Button>
      </div>
      {hasOutput && (
        <div className="flex justify-end">
          <Button type="button" variant="ghost" size="sm" onClick={closeOutput}>
            Close
          </Button>
        </div>
      )}
      {diagnostics.map((item, index) => (
        <p key={`${item.path}-${index}`} role="alert" className="text-sm text-destructive">
          <span className="font-mono">{item.path}</span>: {item.message}
        </p>
      ))}
      {(validation?.warnings ?? []).map((item, index) => (
        <p key={`warning-${item.path}-${index}`} className="text-sm text-amber-700">
          <span className="font-mono">{item.path}</span>: {item.message}
        </p>
      ))}
      {validation?.valid && <ValidatedTemplate validation={validation} />}
    </section>
  );
}
