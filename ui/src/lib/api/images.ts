import type { Transport } from "./client";
import type {
  ImageBuildResult,
  ImageFileContent,
  ImageFilesResult,
  ImageListResult,
  ImageManifest,
  ImagePromptResult,
  ImagePromptTemplate,
  ImageProvenance,
  ImageRemoveResult,
  ImageValidateResult,
} from "../api-types.generated";

export type {
  ImageBuildResult,
  ImageDiagnostic,
  ImageFileEntry,
  ImageManifest,
  ImagePromptTemplate,
  ImageProvenance,
  ImageSummary,
  ImageValidateResult,
} from "../api-types.generated";

// Not in /api/openapi.json: served by internal/api/server.go POST /api/image-imports.
export interface ImageArchiveUpload {
  import_id: string;
  ref: string;
  digest: string;
}

const imageDigestQuery = (digest?: string) =>
  digest ? `?expected_digest=${encodeURIComponent(digest)}` : "";
const imagePath = (ref: string) => `/api/images/${encodeURIComponent(ref)}`;
// Encode each path segment but keep the slashes: the read route is a
// {path...} wildcard, so nested paths must arrive slash-separated.
const encImagePath = (p: string) => p.split("/").map(encodeURIComponent).join("/");

export function imagesApi(t: Transport) {
  return {
    listImages: () => t.get<ImageListResult>("/api/images"),
    validateImageDirectory: (input: { path: string; name: string; tag?: string }) =>
      t.post<ImageValidateResult>("/api/images/validate", input),
    buildImageDirectory: (input: { path: string; name: string; tag?: string }) =>
      t.post<ImageBuildResult>("/api/images/build", input),
    imageManifestGet: (ref: string, expectedDigest?: string) =>
      t.get<ImageManifest>(`${imagePath(ref)}${imageDigestQuery(expectedDigest)}`),
    imageTemplateGet: (ref: string, expectedDigest?: string) =>
      t.get<ImagePromptTemplate>(`${imagePath(ref)}/template${imageDigestQuery(expectedDigest)}`),
    imageProvenanceGet: (ref: string, expectedDigest?: string) =>
      t.get<ImageProvenance>(`${imagePath(ref)}/provenance${imageDigestQuery(expectedDigest)}`),
    imagePromptGet: (ref: string, expectedDigest?: string) =>
      t.get<ImagePromptResult>(`${imagePath(ref)}/prompt${imageDigestQuery(expectedDigest)}`),
    imageFilesList: (ref: string, expectedDigest?: string) =>
      t.get<ImageFilesResult>(`${imagePath(ref)}/files${imageDigestQuery(expectedDigest)}`),
    imageFileRead: (ref: string, path: string, expectedDigest?: string) =>
      t.get<ImageFileContent>(
        `${imagePath(ref)}/files/${encImagePath(path)}${imageDigestQuery(expectedDigest)}`,
      ),
    removeImage: (ref: string) => t.delete<ImageRemoveResult>(imagePath(ref)),
    downloadImageArchive: async (ref: string) =>
      (await t.raw("GET", `${imagePath(ref)}/export`)).blob(),
    uploadImageArchive: async (archive: Blob) => {
      const response = await t.raw("POST", "/api/image-imports", archive);
      return ((await response.json()) as { result: ImageArchiveUpload }).result;
    },
    applyImageArchive: (importID: string, ref?: string) =>
      t.post(`/api/image-imports/${encodeURIComponent(importID)}/apply`, ref ? { ref } : {}),
  };
}
