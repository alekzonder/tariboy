import { useEffect, useState } from "react";
import type { ApiClient, TeamImportOperation, TeamImportPlan } from "@/lib/api";
import { readKey, removeKey, storageKeys, writeKey } from "@/lib/storage";
import { toast } from "sonner";
import { changedImageRefs, previewFromImport, type ImportPreview } from "./teamImportModel";

type OperationRef = { id: string; api: ApiClient };

function storedOperation(api: ApiClient): OperationRef | null {
  const record = readKey(storageKeys.teamImport);
  if (!record) {
    removeKey(storageKeys.teamImport); // drops an unreadable record
    return null;
  }
  return record.hostId === (api.target?.id ?? "") ? { id: record.id, api } : null;
}

function pendingOperation(
  preview: TeamImportPlan,
  steps: TeamImportOperation["steps"],
): TeamImportOperation {
  return {
    import_id: preview.import_id,
    team: preview.team,
    status: "pending",
    steps,
    updated_at: "",
  };
}

// A team import as the page drives it: the editable preview, the server-side
// operation it started, polling while that operation runs, and the import id
// remembered across reloads for the host that started it.
export function useTeamImport(api: ApiClient, load: () => Promise<void>) {
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [operation, setOperation] = useState<TeamImportOperation | null>(null);
  const [operationRef, setOperationRef] = useState<OperationRef | null>(() => storedOperation(api));

  useEffect(() => {
    if (!operationRef) return;
    void operationRef.api.groups
      .getTeamImportOperation(operationRef.id)
      .then(setOperation)
      .catch(() => removeKey(storageKeys.teamImport));
  }, [operationRef]);
  useEffect(() => {
    if (!operationRef || operation?.status !== "running") return;
    const poll = () =>
      void operationRef.api.groups
        .getTeamImportOperation(operationRef.id)
        .then((next) => {
          setOperation(next);
          if (next.status === "complete") {
            removeKey(storageKeys.teamImport);
            void load();
          }
        })
        .catch(() => {});
    const timer = window.setInterval(poll, 500);
    return () => window.clearInterval(timer);
  }, [operationRef, operation?.status, load]);

  const previewYaml = (yaml: string, onPreviewed: () => void) =>
    void api.groups
      .importTeamYAML(yaml)
      .then((result) => {
        setPreview(previewFromImport(result, api));
        setOperationRef({ id: result.import_id, api });
        setOperation(pendingOperation(result, []));
        onPreviewed();
      })
      .catch((error) => toast.error(String(error)));

  const uploadArchive = (file: File) =>
    void api.groups
      .uploadTeamArchive(file)
      .then((result) => {
        setPreview(previewFromImport(result, api));
        setOperationRef({ id: result.import_id, api });
        setOperation(
          pendingOperation(
            result,
            result.images.map((image) => ({
              kind: "image",
              name: image.ref,
              status: "pending",
            })),
          ),
        );
      })
      .catch((error) => toast.error(String(error)));

  const confirm = (current: ImportPreview) => {
    const groups = current.api.groups;
    writeKey(storageKeys.teamImport, {
      id: current.import_id,
      hostId: current.api.target?.id ?? "",
    });
    setOperation((op) => (op ? { ...op, status: "running" } : op));
    void groups
      .applyTeamArchive(current.import_id, {
        refs: changedImageRefs(current.images),
        yaml: current.yaml,
        update_existing: current.updateExisting,
      })
      .then(() => groups.getTeamImportOperation(current.import_id))
      .then((next) => {
        setOperation(next);
        if (next.status === "complete") {
          removeKey(storageKeys.teamImport);
          toast.success("team archive imported");
          setPreview(null);
          void load();
        }
      })
      .catch(async (error) => {
        try {
          setOperation(await groups.getTeamImportOperation(current.import_id));
        } catch {
          /* preserve apply error */
        }
        toast.error(String(error));
      });
  };

  const retry = (ref: OperationRef, failed: TeamImportOperation) => {
    setOperation({ ...failed, status: "running" });
    void ref.api.groups
      .applyTeamArchive(ref.id)
      .then(() => ref.api.groups.getTeamImportOperation(ref.id))
      .then(setOperation)
      .catch(() => void ref.api.groups.getTeamImportOperation(ref.id).then(setOperation));
  };

  return {
    preview,
    setPreview,
    operation,
    operationRef,
    previewYaml,
    uploadArchive,
    confirm,
    retry,
  };
}
