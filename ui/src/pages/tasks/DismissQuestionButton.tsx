import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { errorText } from "@/lib/errors";
import { cn } from "@/lib/utils";
import { writeTask } from "./taskQueries";

/** Closes the customer's open questions on a task without an answer, in any
 *  status. The daemon keeps a workflow's own question. */
export default function DismissQuestionButton({
  taskKey,
  disabled = false,
  className,
  onChanged,
}: {
  taskKey: string;
  disabled?: boolean;
  className?: string;
  onChanged?: () => void;
}) {
  const api = useApi();
  const client = useQueryClient();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const dismiss = async () => {
    setBusy(true);
    setError("");
    try {
      await writeTask(client, api, () => api.tasks.dismissTaskWaits(taskKey));
      onChanged?.();
    } catch (failure) {
      setError(errorText(failure));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <Button
        type="button"
        size="sm"
        variant="ghost"
        className={cn("h-7 shrink-0", className)}
        disabled={disabled || busy}
        onClick={() => void dismiss()}
      >
        Dismiss question
      </Button>
      {error && (
        <span role="alert" className="self-center text-ui text-destructive">
          {error}
        </span>
      )}
    </>
  );
}
