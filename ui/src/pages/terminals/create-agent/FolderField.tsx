import { useEffect, useState } from "react";
import { PathAutocomplete } from "@/components/PathAutocomplete";
import { ApiProvider } from "@/components/ApiProvider";
import { splitPath } from "@/components/splitPath";
import { unresolvedDaemon } from "@/lib/daemons";
import { Label } from "@/components/ui/label";
import type { CreateAgentForm } from "./useCreateAgentForm";

const CHECK_DELAY_MS = 300;

/** Whether the typed folder is missing on the host, from a listing of its
 *  parent. Null while unknown: no listing, an empty field, a managed folder. */
function useFolderMissing(form: Pick<CreateAgentForm, "api" | "draft" | "targetIsCurrent">) {
  const { api, targetIsCurrent } = form;
  const path = form.draft.managedFolder ? "" : form.draft.cwd.trim().replace(/\/+$/, "");
  const [missing, setMissing] = useState<{ path: string; value: boolean } | null>(null);
  useEffect(() => {
    if (!path || !targetIsCurrent) return;
    const { prefix, tail } = splitPath(path);
    if (!tail) return;
    let alive = true;
    const timer = setTimeout(() => {
      api.files
        .fsList(prefix)
        .then((listing) => {
          if (!alive) return;
          const found = listing.entries.some((entry) => entry.dir && entry.name === tail);
          setMissing({ path, value: !found });
        })
        // A parent that cannot be listed says nothing about the folder.
        .catch(() => alive && setMissing(null));
    }, CHECK_DELAY_MS);
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [api, path, targetIsCurrent]);
  return missing?.path === path ? missing.value : null;
}

export function FolderField({
  form,
}: {
  form: Pick<
    CreateAgentForm,
    "api" | "host" | "hosts" | "draft" | "updateDraft" | "target" | "targetIsCurrent"
  >;
}) {
  const { host, hosts, draft, updateDraft, target, targetIsCurrent } = form;
  const missing = useFolderMissing(form);
  return (
    <div className="space-y-1">
      {/* The notice sits beside the label: the folder list opens under the input. */}
      <div className="flex flex-wrap items-baseline justify-between gap-x-3">
        <Label htmlFor="create-agent-cwd">Folder</Label>
        {missing && !draft.createCwd && (
          <p className="text-xs text-muted-foreground">
            This folder does not exist.{" "}
            <button
              type="button"
              className="underline underline-offset-2"
              onClick={() => updateDraft("createCwd", true)}
            >
              Create it
            </button>
          </p>
        )}
        {missing && draft.createCwd && (
          <p className="text-xs text-muted-foreground">The folder will be created.</p>
        )}
      </div>
      {/* The folder is browsed on the host the agent is created on. */}
      <ApiProvider
        target={
          host === ""
            ? null
            : targetIsCurrent && target
              ? target
              : unresolvedDaemon(host, hosts.find((entry) => entry.id === host)?.label)
        }
      >
        <PathAutocomplete
          id="create-agent-cwd"
          value={draft.managedFolder ? "" : draft.cwd}
          onChange={(value) => {
            updateDraft("cwd", value);
            updateDraft("createCwd", false);
            updateDraft("managedFolder", false);
          }}
          placeholder={draft.managedFolder ? "a managed folder" : "~/code/my-app"}
          aria-label="folder"
        />
      </ApiProvider>
    </div>
  );
}
