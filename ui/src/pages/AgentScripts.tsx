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
import { ScriptForm } from "./agentScripts/ScriptForm";
import { ScriptList } from "./agentScripts/ScriptList";
import { useAgentScripts } from "./agentScripts/useAgentScripts";

export default function AgentScripts() {
  const state = useAgentScripts();
  const { remove, setRemove } = state;

  return (
    <div className="mx-auto flex max-w-6xl flex-col gap-6">
      <ScriptForm onLaunch={state.launch} />
      <ScriptList state={state} />
      <AlertDialog
        open={remove !== null}
        onOpenChange={(open) => {
          if (!open) setRemove(null);
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove script?</AlertDialogTitle>
            <AlertDialogDescription>
              Remove {remove?.name} and all of its run history. This cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Keep script</AlertDialogCancel>
            <AlertDialogAction onClick={() => void state.confirmRemove()}>Remove</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
