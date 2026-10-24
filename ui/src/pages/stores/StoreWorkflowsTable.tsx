import type { StoreWorkflow, WorkflowImage } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { latestWorkflowVersion, workflowStatus } from "./storeStatus";

interface StoreWorkflowRowProps {
  workflow: StoreWorkflow;
  workflowImages: WorkflowImage[] | null;
  busy: string;
  buildWorkflow: (workflowName: string) => Promise<void>;
}

function StoreWorkflowRow({
  workflow,
  workflowImages,
  busy,
  buildWorkflow,
}: StoreWorkflowRowProps) {
  const latest = latestWorkflowVersion(workflowImages, workflow.name);
  const status = workflowStatus(workflow, latest);
  return (
    <tr
      className={`border-t ${status === "Update needed" ? "bg-amber-50 dark:bg-amber-950/30" : ""}`}
    >
      <td className="px-3 py-2 font-mono">{workflow.name}</td>
      <td className="px-3 py-2 font-mono text-xs">{workflow.version || "Not specified"}</td>
      <td className="px-3 py-2 font-mono text-xs">
        {latest === null ? "Unknown" : (latest ?? "Not built")}
      </td>
      <td
        className={`px-3 py-2 text-xs ${workflow.error ? "text-destructive" : "text-muted-foreground"}`}
      >
        {status}
      </td>
      <td className="px-3 py-2 text-right">
        <Button
          size="sm"
          disabled={Boolean(busy) || Boolean(workflow.error)}
          onClick={() => void buildWorkflow(workflow.name)}
        >
          {busy === `workflow:${workflow.name}`
            ? `Building ${workflow.name}…`
            : `Build ${workflow.name}`}
        </Button>
      </td>
    </tr>
  );
}

export default function StoreWorkflowsTable({
  workflows,
  ...row
}: Omit<StoreWorkflowRowProps, "workflow"> & { workflows: StoreWorkflow[] | undefined }) {
  return (
    <div className="overflow-x-auto rounded border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
          <tr>
            <th className="px-3 py-2">Workflow</th>
            <th className="px-3 py-2">Source workflow_version</th>
            <th className="px-3 py-2">Latest workflow_version</th>
            <th className="px-3 py-2">Status</th>
            <th className="px-3 py-2" />
          </tr>
        </thead>
        <tbody>
          {(workflows ?? []).map((workflow) => (
            <StoreWorkflowRow key={workflow.name} workflow={workflow} {...row} />
          ))}
          {!workflows?.length && (
            <tr>
              <td colSpan={5} className="px-3 py-8 text-center text-muted-foreground">
                No workflow images in this Store.
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
