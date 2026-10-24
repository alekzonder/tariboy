import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { PathPromptDialog } from "./PathPromptDialog";
import type { useFileActions } from "./useFileActions";

export function FileBrowserDialogs({
  actions,
  canCreate,
  canRename,
}: {
  actions: ReturnType<typeof useFileActions>;
  canCreate: boolean;
  canRename: boolean;
}) {
  const {
    newKind,
    setNewKind,
    rename,
    setRename,
    confirm,
    setConfirm,
    promptValue,
    setPromptValue,
    doCreate,
    submitRename,
  } = actions;
  return (
    <>
      {canCreate && (
        <PathPromptDialog
          open={newKind !== null}
          title={newKind === "dir" ? "New folder" : "New file"}
          label="Path (relative to working directory)"
          value={promptValue}
          onValueChange={setPromptValue}
          submitLabel="Create"
          onOpenChange={(o) => {
            if (!o) setNewKind(null);
          }}
          onSubmit={(v) => newKind && doCreate(newKind, v)}
        />
      )}

      {canRename && (
        <PathPromptDialog
          open={rename !== null}
          title="Rename / move"
          label="New path (relative to working directory)"
          value={promptValue}
          onValueChange={setPromptValue}
          submitLabel="Rename"
          onOpenChange={(o) => {
            if (!o) setRename(null);
          }}
          onSubmit={submitRename}
        />
      )}

      <AlertDialog
        open={confirm !== null}
        onOpenChange={(o) => {
          if (!o) setConfirm(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{confirm?.title}</AlertDialogTitle>
            <AlertDialogDescription>{confirm?.body}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              onClick={() => {
                confirm?.run();
                setConfirm(null);
              }}
            >
              {confirm?.actionLabel}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
