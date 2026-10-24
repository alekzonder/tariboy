import { useSearchParams } from "react-router-dom";
import { Segmented } from "@/components/ui/segmented";
import { withView } from "@/appChrome";

const VIEW_OPTIONS = [
  { value: "agents", label: "Agents" },
  { value: "all", label: "All tasks" },
] as const;

// The "Agents / All tasks" switch of the titlebar.
export function ViewSwitch({ allTasks }: { allTasks: boolean }) {
  const [, setSearchParams] = useSearchParams();
  return (
    <Segmented
      label="View"
      className="ml-2"
      options={VIEW_OPTIONS}
      value={allTasks ? "all" : "agents"}
      onChange={(value) => setSearchParams((current) => withView(current, value))}
    />
  );
}
