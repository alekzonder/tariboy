import { ApiError, type ApiClient } from "@/lib/api";
import { errorText } from "@/lib/errors";
import type { TransferRow } from "./transferStatus";

const isConflict = (error: unknown) => error instanceof ApiError && error.status === 409;

async function uploadRetainedArchive(api: ApiClient, retainedArchive: Blob | null) {
  if (!retainedArchive) throw new Error("transfer archive is no longer available");
  return api.images.uploadImageArchive(retainedArchive);
}

export async function transferToTarget({
  api,
  archive,
  ref,
  retryRef,
  report,
  current,
}: {
  api: ApiClient;
  archive: () => Blob | null;
  ref: string;
  retryRef: string;
  report: (row: TransferRow) => void;
  current: () => boolean;
}): Promise<"transferred" | "failed" | "stale"> {
  try {
    report({ status: "previewing" });
    const preview = await uploadRetainedArchive(api, archive());
    if (!current()) return "stale";
    report({ status: "importing", importID: preview.import_id });
    const result = (await api.images.applyImageArchive(preview.import_id, ref)) as {
      reused?: boolean;
    };
    report({ status: result?.reused ? "already-present" : "completed" });
    return "transferred";
  } catch (error) {
    report({
      status: "failed",
      error: errorText(error),
      ...(isConflict(error) ? { retryRef } : {}),
    });
    return "failed";
  }
}
