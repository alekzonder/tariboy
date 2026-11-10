import { useLocation, useNavigate } from "react-router-dom";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { HostAgents } from "@/lib/aggregate";
import { allHostsTarget, hostScopeTarget } from "@/lib/navRoutes";
import { hostToParam } from "@/lib/terminalsHost";

/** The selector value of "All hosts". A host parameter never starts with NUL. */
const ALL_HOSTS = "\u0000all";

/**
 * The host a page shows, in its header. Only a workspace with two or more
 * hosts has one; with one host nothing about hosts is shown. A pick keeps
 * the page and changes its host. With `allHosts` the first item is
 * **All hosts**, selected while `hostId` is undefined; a pick of it drops
 * `?host=` and the task the query named on its host.
 */
export function HostScopeSelect({
  hosts,
  hostId,
  allHosts = false,
}: {
  hosts: HostAgents[];
  hostId: string | undefined;
  allHosts?: boolean;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  if (hosts.length < 2) return null;
  const value = hostId === undefined ? ALL_HOSTS : hostToParam(hostId);
  return (
    <Select
      value={value}
      onValueChange={(param) => {
        if (param === ALL_HOSTS) {
          navigate(allHostsTarget(location.pathname, location.search));
          return;
        }
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
        {allHosts && (
          <>
            <SelectItem value={ALL_HOSTS}>All hosts</SelectItem>
            <SelectSeparator />
          </>
        )}
        {hosts.map((entry) => (
          <SelectItem key={hostToParam(entry.host.id)} value={hostToParam(entry.host.id)}>
            {entry.host.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}
