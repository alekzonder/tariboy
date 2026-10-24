import type { Transport } from "./client";
import type { PromptLayer } from "./agents";

// One image as listed by GET /api/images (the create form's image combobox).
export interface ImageRow {
  image_version?: string;
  schema_version?: number;
  name: string;
  tag: string;
  digest?: string;
  built_at?: string;
  bare: boolean;
  source?: string;
  exportable?: boolean;
  source_digest?: string;
  source_cwd?: string;
  source_available?: boolean;
  current_agents?: string[];
  pending_agents?: string[];
}

// ---- Images (read-only image detail: manifest / prompt / packed files) ----
// `ref` is name:tag; it rides in the URL path so it is percent-encoded whole.
export interface ImageManifestPlugin {
  name: string;
  version?: string;
}
export interface ImageManifestSkill {
  name: string;
  description: string;
  source: string;
  category: string;
  archive_root: string;
  file_count: number;
  size: number;
  tree_sha256: string;
}
export interface ImageManifestHarness {
  type: string;
  model?: string;
  effort?: string;
  interactive: boolean;
}
export interface ImageManifestPolicy {
  tools_allow?: string[];
  tools_deny?: string[];
}
export interface ImageManifest {
  image_version?: string;
  schema_version: number;
  name: string;
  tag: string;
  digest?: string;
  built_at: string;
  parents: string[] | null;
  plugins: ImageManifestPlugin[] | null;
  skills: ImageManifestSkill[] | null;
  requires_secrets: string[] | null;
  harness?: ImageManifestHarness;
  env: Record<string, string> | null;
  policy?: ImageManifestPolicy;
  layers: PromptLayer[] | null;
  bare?: boolean;
  prompt_template_sha256?: string;
}
export interface ImageTemplateEntry {
  kind: "file" | "runtime";
  runtime?: string;
  source?: string;
  category?: string;
  archive_path?: string;
  size?: number;
  sha256?: string;
}
export interface ImagePromptTemplate {
  schema_version: number;
  entries: ImageTemplateEntry[];
  sha256: string;
}
export interface ImageProvenance {
  ref: string;
  digest?: string;
  source_cwd: string | null;
  built_at?: string;
  source_available: boolean;
}
export interface ImageBuildResult {
  name: string;
  tag: string;
  digest: string;
  layers: number;
}
export interface ImageDiagnostic {
  path: string;
  message: string;
}
export interface ImageValidationResult {
  valid: boolean;
  schema_version: number;
  plugins?: string[];
  skills?: ImageManifestSkill[];
  template?: ImagePromptTemplate | null;
  diagnostics?: ImageDiagnostic[];
  warnings?: ImageDiagnostic[];
}
// One member of an image's tar.gz. The backend returns a FLAT list of every
// member with its full slash-separated path; the image Files tab folds this
// into per-directory listings client-side (the FileBrowser expects listDir).
export interface ImageFileEntry {
  path: string;
  is_dir: boolean;
  size: number;
}
export interface ImageArchiveUpload {
  import_id: string;
  ref: string;
  digest: string;
}

const imageDigestQuery = (digest?: string) => digest ? `?expected_digest=${encodeURIComponent(digest)}` : "";
const imagePath = (ref: string) => `/api/images/${encodeURIComponent(ref)}`;
// Encode each path segment but keep the slashes: the read route is a
// {path...} wildcard, so nested paths must arrive slash-separated.
const encImagePath = (p: string) => p.split("/").map(encodeURIComponent).join("/");

export function imagesApi(t: Transport) {
  return {
    listImages: () => t.get<{ images: ImageRow[]; count: number }>("/api/images"),
    validateImageDirectory: (input: { path: string; name: string; tag?: string }) =>
      t.post<ImageValidationResult>("/api/images/validate", input),
    buildImageDirectory: (input: { path: string; name: string; tag?: string }) =>
      t.post<ImageBuildResult>("/api/images/build", input),
    imageManifestGet: (ref: string, expectedDigest?: string) =>
      t.get<ImageManifest>(`${imagePath(ref)}${imageDigestQuery(expectedDigest)}`),
    imageTemplateGet: (ref: string, expectedDigest?: string) =>
      t.get<ImagePromptTemplate>(`${imagePath(ref)}/template${imageDigestQuery(expectedDigest)}`),
    imageProvenanceGet: (ref: string, expectedDigest?: string) =>
      t.get<ImageProvenance>(`${imagePath(ref)}/provenance${imageDigestQuery(expectedDigest)}`),
    imagePromptGet: (ref: string, expectedDigest?: string) =>
      t.get<{ prompt: string }>(`${imagePath(ref)}/prompt${imageDigestQuery(expectedDigest)}`),
    imageFilesList: (ref: string, expectedDigest?: string) =>
      t.get<{ files: ImageFileEntry[] | null; count: number }>(`${imagePath(ref)}/files${imageDigestQuery(expectedDigest)}`),
    imageFileRead: (ref: string, path: string, expectedDigest?: string) =>
      t.get<{ path: string; content: string }>(`${imagePath(ref)}/files/${encImagePath(path)}${imageDigestQuery(expectedDigest)}`),
    removeImage: (ref: string) => t.delete<{ removed: string }>(imagePath(ref)),
    downloadImageArchive: async (ref: string) => (await t.raw("GET", `${imagePath(ref)}/export`)).blob(),
    uploadImageArchive: async (archive: Blob) => {
      const response = await t.raw("POST", "/api/image-imports", archive);
      return ((await response.json()) as { result: ImageArchiveUpload }).result;
    },
    applyImageArchive: (importID: string, ref?: string) =>
      t.post(`/api/image-imports/${encodeURIComponent(importID)}/apply`, ref ? { ref } : {}),
  };
}
export type ImagesApi = ReturnType<typeof imagesApi>;
