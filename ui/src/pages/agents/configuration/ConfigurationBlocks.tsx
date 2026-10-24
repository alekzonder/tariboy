import { Input } from "@/components/ui/input";
import { GoalHelp } from "@/components/GoalHelp";
import { fieldOfKey, type ConfigBlock, type ConfigField } from "./registry";
import type { ConfigDraft } from "./useConfigDraft";
import type { ConfigSources } from "./useConfigSources";
import { ConfigBlockCard, DraftField, ReadOnly } from "./ConfigControls";
import { GeneralBlock } from "./GeneralBlock";
import { RuntimeBlock } from "./RuntimeBlock";
import { BudgetsBlock } from "./BudgetsBlock";
import { EnvironmentBlock } from "./EnvironmentBlock";
import { RetentionBlock } from "./RetentionBlock";

const field = (key: string) => fieldOfKey(key) as ConfigField;

export interface BlockProps {
  block: ConfigBlock;
  draft: ConfigDraft;
  matched: Set<string>;
  dirty: number;
  sources: ConfigSources;
  name: string;
  stopped: boolean;
  refresh: () => void;
}

export function ConfigurationBlocks(props: BlockProps) {
  const { block, draft, matched, dirty } = props;
  const plain = (key: string) => (
    <DraftField field={field(key)} draft={draft} matched={matched.has(key)} />
  );
  let body;
  switch (block.id) {
    case "general":
      body = <GeneralBlock {...props} />;
      break;
    case "runtime":
      body = <RuntimeBlock {...props} />;
      break;
    case "loop":
      body = (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            {plain("interval")}
            {plain("timeout")}
            {plain("hard-timeout")}
          </div>
          <div className="space-y-1">
            {plain("schedule")}
            <ScheduleNextRun nextAt={props.sources.view?.schedule_next_at} />
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {plain("on-timeout")}
            {plain("on-error")}
          </div>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {plain("max-idle")}
            {plain("ai-stall-timeout")}
          </div>
        </>
      );
      break;
    case "goal":
      body = (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {plain("goal-enabled")}
            {plain("goal-wait-customer-timeout")}
            {plain("goal-delivery-cooldown")}
          </div>
          <ReadOnly draft={draft} matched={matched} fieldKey="current-goal-task">
            <Input
              id="current-goal-task"
              value={props.sources.view?.current_goal_task_key || "No current goal"}
              disabled
              aria-describedby="current-goal-task-help"
            />
          </ReadOnly>
        </>
      );
      break;
    case "messages":
      body = (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          {plain("messages-batch")}
          {plain("messages-max-queue")}
        </div>
      );
      break;
    case "budgets":
      body = <BudgetsBlock {...props} />;
      break;
    case "environment":
      body = <EnvironmentBlock {...props} />;
      break;
    case "retention":
      body = <RetentionBlock {...props} />;
      break;
  }
  return (
    <ConfigBlockCard
      block={block}
      dirtyCount={dirty}
      action={block.id === "goal" ? <GoalHelp /> : undefined}
    >
      {body}
    </ConfigBlockCard>
  );
}

// The daemon computes the next start from the saved rule, so this line
// follows Save, not the unsaved draft.
function ScheduleNextRun({ nextAt }: { nextAt?: string }) {
  return (
    <p data-testid="schedule-next-run" className="text-xs text-muted-foreground">
      {nextAt ? `Next run: ${new Date(nextAt).toLocaleString()}` : "No scheduled run"}
    </p>
  );
}
