import type { Transport } from "./client";

interface ImageBuildRecord {
  ref: string;
  digest: string;
  built_at: string;
}

export interface ImageSourceSummary {
  schema_version: number;
  name: string;
  created_at: string;
  updated_at: string;
  last_build?: ImageBuildRecord;
}

export type ImageSource = ImageSourceSummary;

export interface ImageSourceFile {
  path: string;
  size: number;
}

interface ImageSourceDiagnostic {
  path: string;
  field?: string;
  message: string;
}

export interface CreateImageSource {
  name: string;
  prompt?: string;
}

export interface ImageSourceBuildResult {
  ref: string;
  digest: string;
  built_at: string;
  layers: number;
}

export interface ImageValidation {
  valid: boolean;
  diagnostics: ImageSourceDiagnostic[];
}

const sourcePath = (name: string) => `/api/image-sources/${encodeURIComponent(name)}`;
const sourceFilePath = (name: string, path: string) =>
  `${sourcePath(name)}/files/${path.split("/").map(encodeURIComponent).join("/")}`;

export function imageSourcesApi(t: Transport) {
  return {
    listImageSources: () =>
      t.get<{ sources: ImageSourceSummary[]; count: number }>("/api/image-sources"),
    createImageSource: (input: CreateImageSource) =>
      t.post<ImageSource>("/api/image-sources", input),
    getImageSource: (name: string) => t.get<ImageSource>(sourcePath(name)),
    deleteImageSource: (name: string) => t.delete<{ removed: string }>(sourcePath(name)),
    listImageSourceFiles: (name: string) =>
      t.get<{ files: ImageSourceFile[]; count: number }>(`${sourcePath(name)}/files`),
    getImageSourceFile: (name: string, path: string) =>
      t.get<{ path: string; content: string }>(sourceFilePath(name, path)),
    putImageSourceFile: (name: string, path: string, content: string) =>
      t.put<{ path: string; saved: boolean }>(sourceFilePath(name, path), { content }),
    validateImageSource: (name: string) => t.post<ImageValidation>(`${sourcePath(name)}/validate`),
    buildImageSource: (name: string, tag: string) =>
      t.post<ImageSourceBuildResult>(`${sourcePath(name)}/build`, { tag }),
  };
}
