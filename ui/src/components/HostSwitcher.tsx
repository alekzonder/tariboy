import { useDaemons } from "./DaemonProvider";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";

const LOCAL = "__local__"; // sentinel for the same-origin "This daemon" entry

// HostSwitcher picks the active daemon. The first entry is always the local
// same-origin daemon (activeId ""); registered daemons follow. Labels only —
// tokens are never shown.
export function HostSwitcher() {
  const { daemons, activeId, select } = useDaemons();
  const value = activeId || LOCAL;
  return (
    <Select value={value} onValueChange={(v) => select(v === LOCAL ? "" : v)}>
      <SelectTrigger className="h-8 w-[180px] text-sm">
        <SelectValue placeholder="This daemon (local)" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value={LOCAL}>This daemon (local)</SelectItem>
        {daemons.map((d) => (
          <SelectItem key={d.id} value={d.id}>
            {d.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
