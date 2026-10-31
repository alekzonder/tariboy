import { useLocation, useNavigate } from "react-router-dom";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { HostAgents } from "@/lib/aggregate";
import { hostScopeTarget } from "@/lib/navRoutes";
import { hostToParam } from "@/lib/terminalsHost";

/**
 * The host a page shows, in its header. Only a workspace with two or more
 * hosts has one; with one host nothing about hosts is shown. A pick keeps
 * the page and changes its host.
 */
export function HostScopeSelect({ hosts, hostId }: { hosts: HostAgents[]; hostId: string }) {
  const navigate = useNavigate();
  const location = useLocation();
  if (hosts.length < 2) return null;
  return (
    <Select
      value={hostToParam(hostId)}
      onValueChange={(param) => {
        const next = hosts.find((entry) => hostToParam(entry.host.id) === param);
        if (next) navigate(hostScopeTarget(location.pathname, location.search, next.host.id));
      }}
    >
      <SelectTrigger
        aria-label="Host"
        size="sm"
        className="w-auto max-w-48 shrink-0 gap-1.5 text-body-sm"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {hosts.map((entry) => (
          <SelectItem key={hostToParam(entry.host.id)} value={hostToParam(entry.host.id)}>
            {entry.host.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
