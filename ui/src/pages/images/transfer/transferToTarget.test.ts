import { describe, expect, it, vi } from "vitest";
import { ApiError, type ApiClient } from "@/lib/api";
import { errorText } from "@/lib/errors";
import type { TransferRow } from "./transferStatus";
import { transferToTarget } from "./transferToTarget";

const archive = new Blob(["archive"]);

function destination(apply: () => Promise<unknown>) {
  const uploadImageArchive = vi.fn(async () => ({ import_id: "import-1", ref: "demo:1" }));
  const applyImageArchive = vi.fn(apply);
  const api = { images: { uploadImageArchive, applyImageArchive } } as unknown as ApiClient;
  return { api, uploadImageArchive, applyImageArchive };
}

function attempt(
  api: ApiClient,
  overrides: { current?: () => boolean; archive?: Blob | null } = {},
) {
  const rows: TransferRow[] = [];
  return {
    rows,
    input: {
      api,
      archive: () => (overrides.archive === undefined ? archive : overrides.archive),
      ref: "demo:2",
      retryRef: " demo:2 ",
      report: (row: TransferRow) => rows.push(row),
      current: overrides.current ?? (() => true),
    },
  };
}

describe("transferToTarget", () => {
  it("uploads the archive and applies the import under the requested ref", async () => {
    const target = destination(async () => ({}));
    const { rows, input } = attempt(target.api);

    await expect(transferToTarget(input)).resolves.toBe("transferred");

    expect(target.uploadImageArchive).toHaveBeenCalledWith(archive);
    expect(target.applyImageArchive).toHaveBeenCalledWith("import-1", "demo:2");
    expect(rows).toEqual([
      { status: "previewing" },
      { status: "importing", importID: "import-1" },
      { status: "completed" },
    ]);
  });

  it("reports a reused image as already present", async () => {
    const { rows, input } = attempt(destination(async () => ({ reused: true })).api);

    await expect(transferToTarget(input)).resolves.toBe("transferred");

    expect(rows.at(-1)).toEqual({ status: "already-present" });
  });

  it("keeps the retry ref when the destination reports a conflict", async () => {
    const conflict = new ApiError(409, "image_exists", "tag exists");
    const { rows, input } = attempt(destination(() => Promise.reject(conflict)).api);

    await expect(transferToTarget(input)).resolves.toBe("failed");

    expect(rows.at(-1)).toEqual({
      status: "failed",
      error: errorText(conflict),
      retryRef: " demo:2 ",
    });
  });

  it("fails without a retry ref on any other error", async () => {
    const failure = new ApiError(500, "internal", "broken");
    const { rows, input } = attempt(destination(() => Promise.reject(failure)).api);

    await expect(transferToTarget(input)).resolves.toBe("failed");

    expect(rows.at(-1)).toEqual({ status: "failed", error: errorText(failure) });
  });

  it("fails when the retained archive is gone", async () => {
    const target = destination(async () => ({}));
    const { rows, input } = attempt(target.api, { archive: null });

    await expect(transferToTarget(input)).resolves.toBe("failed");

    expect(target.uploadImageArchive).not.toHaveBeenCalled();
    expect(rows.at(-1)).toEqual({
      status: "failed",
      error: "transfer archive is no longer available",
    });
  });

  it("stops without importing when the run became stale during upload", async () => {
    const target = destination(async () => ({}));
    const { rows, input } = attempt(target.api, { current: () => false });

    await expect(transferToTarget(input)).resolves.toBe("stale");

    expect(target.applyImageArchive).not.toHaveBeenCalled();
    expect(rows).toEqual([{ status: "previewing" }]);
  });
});
