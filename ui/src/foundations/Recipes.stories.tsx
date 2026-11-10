import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import {
  CELL,
  COUNT,
  DANGER_FILL,
  EMPTY,
  FIELD,
  FIELD_MONO,
  ISLAND,
  LABEL,
  MONO,
  PRIMARY_FILL,
  QUIET_ACTION,
  ROW,
  STICKY_CELL,
  SUCCESS_FILL,
  WARNING_FILL,
} from "@/components/ui/recipes";
import { cn } from "@/lib/utils";

// The recipes of components/ui/recipes.ts, each on the element it is made for.
const meta = {
  title: "Foundations/Recipes",
  parameters: { layout: "fullscreen" },
} satisfies Meta;
export default meta;

type Story = StoryObj;

const NAME = "w-32 shrink-0 font-mono text-caption text-muted-foreground";

/** A section of a detail panel: label, counter, quiet action, rows, fields. */
export const Panel: Story = {
  render: () => (
    <div className="bg-background p-6 text-foreground">
      <div className={cn(ISLAND, "flex w-140 flex-col gap-4 p-4")}>
        <div className="flex items-center gap-2">
          <span className={LABEL}>Dependencies</span>
          <span className={COUNT}>2</span>
          <Button type="button" variant="ghost" className={cn(QUIET_ACTION, "ml-auto")}>
            Add
          </Button>
        </div>
        <ul className="flex flex-col">
          <li className={ROW}>
            <span className={MONO}>TB-142</span>
            <span className="min-w-0 truncate text-body">Ship the skills-tree validator</span>
          </li>
          <li className={ROW}>
            <span className={MONO}>TB-98</span>
            <span className="min-w-0 truncate text-body">Remote provision smoke fails</span>
          </li>
        </ul>
        <span className={EMPTY}>Nothing blocks this task.</span>
        <label className="flex flex-col gap-1">
          <span className={LABEL}>Manual block reason</span>
          <Input className={FIELD} defaultValue="Waiting for the release window" />
        </label>
        <label className="flex flex-col gap-1">
          <span className={LABEL}>Pull request</span>
          <Input className={cn(FIELD, FIELD_MONO)} defaultValue="tariboy/tariboy pull 365" />
        </label>
        <label className="flex flex-col gap-1">
          <span className={LABEL}>Priority</span>
          <NativeSelect defaultValue="P2" aria-label="Priority">
            <option>P1</option>
            <option>P2</option>
            <option>P3</option>
          </NativeSelect>
        </label>
      </div>
    </div>
  ),
};

/** The four fills of a banner, and the names of the recipes above. */
export const Fills: Story = {
  render: () => (
    <div className="flex flex-col gap-3 bg-card p-6 text-body text-foreground">
      {[
        ["PRIMARY_FILL", PRIMARY_FILL, "builder asks: which image builds the docs?"],
        ["DANGER_FILL", DANGER_FILL, "The workflow is paused: the budget is exhausted."],
        ["WARNING_FILL", WARNING_FILL, "worker:v1 has a newer digest in the store."],
        ["SUCCESS_FILL", SUCCESS_FILL, "build-01 is provisioned and connected."],
      ].map(([name, fill, text]) => (
        <div key={name} className="flex items-center gap-3">
          <span className={NAME}>{name}</span>
          <span className={cn("rounded-control px-2.5 py-2 font-medium", fill)}>{text}</span>
        </div>
      ))}
    </div>
  ),
};

/** A dense data table with a pinned first column. */
export const Table: Story = {
  render: () => (
    <div className="bg-card p-6 text-foreground">
      <div className="w-96 overflow-x-auto rounded-card border border-border">
        <table className="w-full text-ui">
          <thead className="bg-subtle text-muted-foreground">
            <tr>
              <th className={cn(CELL, STICKY_CELL, "text-left font-medium")}>Agent</th>
              <th className={cn(CELL, "text-right font-medium")}>Iterations</th>
              <th className={cn(CELL, "text-right font-medium")}>Cost, USD</th>
              <th className={cn(CELL, "text-right font-medium")}>Median duration</th>
            </tr>
          </thead>
          <tbody>
            {[
              ["builder", "128", "41.20", "00:12:41"],
              ["reviewer", "64", "9.85", "00:03:07"],
            ].map(([agent, runs, cost, time]) => (
              <tr key={agent} className="border-t border-border">
                <td className={cn(CELL, STICKY_CELL, "font-medium")}>{agent}</td>
                <td className={cn(CELL, MONO, "text-right")}>{runs}</td>
                <td className={cn(CELL, MONO, "text-right")}>{cost}</td>
                <td className={cn(CELL, MONO, "text-right")}>{time}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  ),
};
