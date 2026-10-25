import type {
  ImageFileContent,
  ImageRemoveResult,
  ImageSource,
  ImageSourceBuildResult,
  ImageSourceFilesResult,
  ImageSourceListResult,
  ImageSourceSaveResult,
  ImageSourceValidateResult,
} from "../api-types.generated";
import type { Transport } from "./client";

export type {
  ImageSource,
  ImageSourceBuildResult,
  ImageSourceValidateResult,
} from "../api-types.generated";

export interface CreateImageSource {
  name: string;
  prompt?: string;
}

const sourcePath = (name: string) => `/api/image-sources/${encodeURIComponent(name)}`;
const sourceFilePath = (name: string, path: string) =>
  `${sourcePath(name)}/files/${path.split("/").map(encodeURIComponent).join("/")}`;

export function imageSourcesApi(t: Transport) {
  return {
    listImageSources: () => t.get<ImageSourceListResult>("/api/image-sources"),
    createImageSource: (input: CreateImageSource) =>
      t.post<ImageSource>("/api/image-sources", input),
    getImageSource: (name: string) => t.get<ImageSource>(sourcePath(name)),
    deleteImageSource: (name: string) => t.delete<ImageRemoveResult>(sourcePath(name)),
    listImageSourceFiles: (name: string) =>
      t.get<ImageSourceFilesResult>(`${sourcePath(name)}/files`),
    getImageSourceFile: (name: string, path: string) =>
      t.get<ImageFileContent>(sourceFilePath(name, path)),
    putImageSourceFile: (name: string, path: string, content: string) =>
      t.put<ImageSourceSaveResult>(sourceFilePath(name, path), { content }),
    validateImageSource: (name: string) =>
      t.post<ImageSourceValidateResult>(`${sourcePath(name)}/validate`),
    buildImageSource: (name: string, tag: string) =>
      t.post<ImageSourceBuildResult>(`${sourcePath(name)}/build`, { tag }),
  };
}
