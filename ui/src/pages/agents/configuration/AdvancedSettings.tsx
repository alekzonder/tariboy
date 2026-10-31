import { ChevronRight } from "lucide-react";
import { Link } from "react-router-dom";
import { Input } from "@/components/ui/input";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { GoalHelp } from "@/components/GoalHelp";
import { useAgentStatus } from "@/lib/agent";
import { ClearInbox } from "../ClearInbox";
import { DraftField, ReadOnly, SettingsSection, type SectionProps } from "./ConfigControls";
import { unsaved } from "./draftFieldModel";
import { EnvironmentBlock } from "./EnvironmentBlock";
import { GeneralBlock } from "./GeneralBlock";
import { RetentionBlock } from "./RetentionBlock";
import { fieldOfKey, type ConfigField } from "./registry";

const field = (key: string) => fieldOfKey(key) as ConfigField;

// The daemon computes the next start from the saved rule, so this line
// follows Save, not the unsaved draft.
function ScheduleNextRun({ nextAt }: { nextAt?: string }) {
  return (
    <p data-testid="schedule-next-run" className="text-xs text-muted-foreground">
      {nextAt ? `Next run: ${new Date(nextAt).toLocaleString()}` : "No scheduled run"}
    </p>
  );
}

function ScheduleSection({ draft, sources }: SectionProps) {
  const plain = (key: string) => <DraftField field={field(key)} draft={draft} />;
  return (
    <SettingsSection id="schedule" title="Schedule and timeouts" level={4}>
      <div className="space-y-1">
        {plain("schedule")}
        <ScheduleNextRun nextAt={sources.view?.schedule_next_at} />
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        {plain("hard-timeout")}
        {plain("on-timeout")}
        {plain("on-error")}
      </div>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {plain("max-idle")}
        {plain("ai-stall-timeout")}
      </div>
    </SettingsSection>
  );
}

function GoalSection({ draft, sources }: SectionProps) {
  return (
    <SettingsSection id="goal" title="Goal" level={4} action={<GoalHelp />}>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <DraftField field={field("goal-enabled")} draft={draft} />
        <DraftField field={field("goal-wait-customer-timeout")} draft={draft} />
        <DraftField field={field("goal-delivery-cooldown")} draft={draft} />
      </div>
      <ReadOnly draft={draft} fieldKey="current-goal-task">
        <Input
          id="current-goal-task"
          value={sources.view?.current_goal_task_key || "No current goal"}
          disabled
          aria-describedby="current-goal-task-help"
        />
      </ReadOnly>
    </SettingsSection>
  );
}

function MailSection({ draft, name }: SectionProps) {
  const { status } = useAgentStatus();
  return (
    <SettingsSection id="mail" title="Mail" level={4}>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <DraftField field={field("messages-batch")} draft={draft} />
        <DraftField field={field("messages-max-queue")} draft={draft} />
      </div>
      <ClearInbox name={name} disabled={(status?.messages_pending ?? 0) === 0} />
    </SettingsSection>
  );
}

function BudgetsSection({ draft, sources }: SectionProps) {
  const budget = sources.view?.budget;
  if (!budget) return null;
  const spent = budget.hour_spent_usd.toFixed(2);
  const limit = Number(draft.values["budget-hour"] ?? 0);
  const helper =
    limit === 0 ? `Spent ${spent} · Unlimited` : `Spent ${spent} of ${limit.toFixed(2)}`;
  return (
    <SettingsSection id="budgets" title="Hour budget (USD)" level={4}>
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <DraftField field={{ ...field("budget-hour"), helper }} draft={draft} />
      </div>
    </SettingsSection>
  );
}

const OTHER_VIEWS = [
  ["prompt", "Prompt preview"],
  ["context", "Context"],
  ["files", "Files"],
  ["scripts", "Scripts"],
  ["audit", "Full audit"],
  ["usage", "Raw usage"],
  ["raw", "Raw settings"],
] as const;

function OtherViews({ base }: { base: string }) {
  return (
    <SettingsSection id="views" title="Other views" level={4}>
      <nav aria-label="Other views" className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
        <Link className="text-primary hover:underline" to={`${base}/mailbox`}>
          Open mailbox
        </Link>
        {OTHER_VIEWS.map(([view, label]) => (
          <Link
            key={view}
            className="text-primary hover:underline"
            to={`${base}/advanced?view=${view}`}
          >
            {label}
          </Link>
        ))}
      </nav>
    </SettingsSection>
  );
}

// AdvancedSettings is the one disclosure for every setting outside the four
// groups. It starts closed; drafts inside it live in the page, so closing it
// never drops them, and its trigger counts them.
export function AdvancedSettings(
  props: SectionProps & {
    base: string;
    open: boolean;
    onOpenChange: (open: boolean) => void;
    dirty: number;
    hidden: boolean;
  },
) {
  const { base, open, onOpenChange, dirty, hidden, ...section } = props;
  return (
    <Collapsible open={open} onOpenChange={onOpenChange} hidden={hidden} className="border-t pt-3">
      <CollapsibleTrigger className="group flex items-center gap-2 text-sm font-semibold">
        <ChevronRight
          aria-hidden="true"
          className="size-4 transition-transform group-data-[state=open]:rotate-90"
        />
        Advanced
        {dirty > 0 && <span className="text-xs font-medium text-primary">{unsaved(dirty)}</span>}
      </CollapsibleTrigger>
      <CollapsibleContent
        forceMount
        hidden={!open}
        className={open ? "mt-3 space-y-5 pl-6" : "hidden"}
      >
        <SettingsSection id="image" title="Image" level={4}>
          <GeneralBlock {...section} />
        </SettingsSection>
        <ScheduleSection {...section} />
        <GoalSection {...section} />
        <MailSection {...section} />
        <BudgetsSection {...section} />
        <SettingsSection id="environment" title="Environment" level={4}>
          <EnvironmentBlock {...section} />
        </SettingsSection>
        <SettingsSection id="retention" title="Retention and cleanup" level={4}>
          <RetentionBlock {...section} />
        </SettingsSection>
        <OtherViews base={base} />
      </CollapsibleContent>
    </Collapsible>
  );
}
