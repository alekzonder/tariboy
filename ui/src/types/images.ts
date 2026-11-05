export type {
  ImageDiagnostic,
  ImageFileEntry,
  ImageManifest,
  ImagePromptTemplate,
  ImageProvenance,
  ImageSummary,
  ImageValidateResult,
  ManifestRole,
} from "@/lib/api-types.generated";

// Not in /api/openapi.json: served by internal/api/server.go POST /api/image-imports.
export interface ImageArchiveUpload {
  import_id: string;
  ref: string;
  digest: string;
}
