import { apiOn, resolveTarget, type ApiTarget } from "./api";

export interface ImageBuildRecord {
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

export interface ImageDiagnostic {
  path: string;
  field?: string;
  message: string;
}

export interface CreateImageSource {
  name: string;
  from?: string;
  harness?: string;
  model?: string;
  effort?: string;
  interactive?: boolean;
  capabilities?: string[];
  prompt?: string;
}

export interface ImageBuildResult {
  ref: string;
  digest: string;
  built_at: string;
  layers: number;
}

export interface ImageValidation {
  valid: boolean;
  diagnostics: ImageDiagnostic[];
}

const sourcePath = (name: string) => `/api/image-sources/${encodeURIComponent(name)}`;
const sourceFilePath = (name: string, path: string) =>
  `${sourcePath(name)}/files/${path.split("/").map(encodeURIComponent).join("/")}`;

export const listImageSources = (target?: ApiTarget) =>
  apiOn<{ sources: ImageSourceSummary[]; count: number }>(
    resolveTarget(target), "GET", "/api/image-sources",
  );

export const createImageSource = (input: CreateImageSource, target?: ApiTarget) =>
  apiOn<ImageSource>(resolveTarget(target), "POST", "/api/image-sources", input);

export const getImageSource = (name: string, target?: ApiTarget) =>
  apiOn<ImageSource>(resolveTarget(target), "GET", sourcePath(name));

export const deleteImageSource = (name: string, target?: ApiTarget) =>
  apiOn<{ removed: string }>(resolveTarget(target), "DELETE", sourcePath(name));

export const listImageSourceFiles = (name: string, target?: ApiTarget) =>
  apiOn<{ files: ImageSourceFile[]; count: number }>(
    resolveTarget(target), "GET", `${sourcePath(name)}/files`,
  );

export const getImageSourceFile = (name: string, path: string, target?: ApiTarget) =>
  apiOn<{ path: string; content: string }>(
    resolveTarget(target), "GET", sourceFilePath(name, path),
  );

export const putImageSourceFile = (
  name: string,
  path: string,
  content: string,
  target?: ApiTarget,
) => apiOn<{ path: string; saved: boolean }>(
  resolveTarget(target), "PUT", sourceFilePath(name, path), { content },
);

export const validateImageSource = (name: string, target?: ApiTarget) =>
  apiOn<ImageValidation>(resolveTarget(target), "POST", `${sourcePath(name)}/validate`);

export const buildImageSource = (name: string, tag: string, target?: ApiTarget) =>
  apiOn<ImageBuildResult>(
    resolveTarget(target), "POST", `${sourcePath(name)}/build`, { tag },
  );
