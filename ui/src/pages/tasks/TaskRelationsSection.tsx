import { X } from "lucide-react";
import { errorText } from "@/lib/errors";
import type { TaskDetail, TaskRelationType } from "@/lib/api";
import { flexibleStatusLabel, taskTone } from "@/lib/statusTone";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { StatusPill } from "@/components/ui/status";
import { SectionHeading } from "./TaskDetailControls";
import { DANGER_FILL, EMPTY, FIELD, FIELD_MONO, ROW, SelectShell } from "./panelStyles";
import type { useTaskDetailRelations } from "./useTaskDetailRelations";

export function TaskRelationsSection({
  detail,
  relations,
}: {
  detail: TaskDetail;
  relations: ReturnType<typeof useTaskDetailRelations>;
}) {
  const task = detail.task;
  const {
    relationType,
    setRelationType,
    relationTarget,
    setRelationTarget,
    relationMutation,
    submitRelation,
  } = relations;
  return (
    <section className="flex min-w-0 flex-col gap-1.5">
      <SectionHeading label="Dependencies" count={detail.relations.length} />
      {detail.relations.length === 0 && <span className={EMPTY}>Nothing blocks this task.</span>}
      {detail.relations.map((relation) => {
        const source = relation.source_key === task.key;
        const other = source ? relation.target_key : relation.source_key;
        const otherTitle = source ? relation.target_title : relation.source_title;
        const otherStatus = source ? relation.target_status : relation.source_status;
        return (
          <div key={relation.id} className={ROW}>
            <span
              className={cn(
                "inline-flex h-[19px] shrink-0 items-center rounded-[6px] px-[7px] text-[11px]",
                relation.type === "blocks"
                  ? cn(DANGER_FILL, "font-medium")
                  : "bg-muted text-muted-foreground",
              )}
            >
              {relation.type}
            </span>
            {/* The key is underlined only as far as it reads; the title
                takes the rest of the row and the pill closes it, so a
                dependency says what it is without being opened. */}
            <span className="shrink-0 border-b border-dotted border-border font-mono text-[11.5px] font-medium tabular-nums">
              {other}
            </span>
            <span className="min-w-0 flex-1 truncate text-[12.5px] text-muted-foreground">
              {otherTitle}
            </span>
            {otherStatus && (
              <StatusPill tone={taskTone(otherStatus)}>
                {flexibleStatusLabel(otherStatus)}
              </StatusPill>
            )}
            <Button
              type="button"
              variant="ghost"
              size="icon-xs"
              aria-label={`Remove relation to ${other}`}
              className="size-6 shrink-0 rounded-[7px] text-muted-foreground hover:bg-accent hover:text-destructive"
              onClick={() => relationMutation.mutate({ id: relation.id })}
            >
              <X className="size-3" />
            </Button>
          </div>
        );
      })}
      <form
        className="flex items-center gap-1.5 pt-0.5"
        onSubmit={(event) => {
          event.preventDefault();
          submitRelation();
        }}
      >
        <SelectShell
          aria-label="Relation type"
          value={relationType}
          className="h-[26px] w-auto text-[12px] md:text-[12px]"
          onChange={(event) => setRelationType(event.target.value as TaskRelationType)}
        >
          <option value="blocks">Blocks</option>
          <option value="related">Related</option>
        </SelectShell>
        <Input
          name="target_key"
          aria-label="Related task key"
          placeholder="TEST-2"
          value={relationTarget}
          className={cn(FIELD, FIELD_MONO, "h-[26px] w-[118px]")}
          onChange={(event) => setRelationTarget(event.target.value)}
        />
        <Button
          type="button"
          variant="ghost"
          onClick={submitRelation}
          disabled={relationMutation.isPending || !relationTarget.trim()}
          className="h-[26px] rounded-[7px] bg-accent px-2.5 text-[12px] hover:bg-secondary"
        >
          Add relation
        </Button>
      </form>
      {relationMutation.error && (
        <p role="alert" className="text-[12px] text-status-failed">
          {errorText(relationMutation.error)}
        </p>
      )}
    </section>
  );
}
