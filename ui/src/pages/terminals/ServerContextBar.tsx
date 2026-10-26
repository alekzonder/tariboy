import { NavLink } from "react-router-dom";
import { ApiProvider } from "@/components/ApiProvider";
import { useApi } from "@/components/apiContext";
import { useActiveLinkIntoView } from "@/hooks/useActiveLinkIntoView";
import { useApiQuery } from "@/hooks/useApiQuery";
import { serverPath, targetFor, type ServerSection } from "@/lib/terminalsHost";
import { cn } from "@/lib/utils";
import { mailQueries, useLiveMail } from "@/pages/mailbox/mailQueries";

const SECTIONS: Array<{ section: ServerSection; label: string }> = [
  { section: "mail", label: "Mailbox" },
  { section: "tasks", label: "Tasks" },
  { section: "activity", label: "Activity" },
  { section: "images", label: "Images" },
  { section: "workflows", label: "Workflow images" },
  { section: "stores", label: "Stores" },
  { section: "settings", label: "Settings" },
];

/** The customer's open letters on this server, kept live by the mail socket. */
function OpenMailCount() {
  const api = useApi();
  useLiveMail("");
  const open = useApiQuery(mailQueries.unread(api, "")).data?.open ?? 0;
  if (open === 0) return null;
  const label = `${open} open letter${open === 1 ? "" : "s"}`;
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className="ml-1.5 rounded-full bg-primary px-1.5 text-caption font-medium leading-4 text-primary-foreground"
    >
      {open}
    </span>
  );
}

/** The island's header for server-scoped views. Same strip as the agent tabs —
 *  34px, a 2px --primary underline on the active section — so the two ways of
 *  entering the island read as one control surface. */
export function ServerContextBar({ hostId, label }: { hostId: string; label: string }) {
  const strip = useActiveLinkIntoView<HTMLElement>();
  return (
    <nav
      ref={strip}
      aria-label="Server workspace"
      className="scrollbar-none flex shrink-0 items-center gap-0.5 overflow-x-auto border-b px-3"
    >
      <span className="mr-2 flex max-w-[40vw] min-w-0 shrink-0 items-baseline gap-1.5 text-body-sm">
        <span className="text-muted-foreground">Server:</span>
        <span className="truncate font-medium" title={label}>
          {label}
        </span>
      </span>
      {SECTIONS.map(({ section, label: sectionLabel }) => (
        <NavLink
          key={section}
          to={serverPath(hostId, section)}
          className={({ isActive }) =>
            cn(
              "flex h-8.5 shrink-0 items-center border-b-2 border-transparent px-2.5 text-body whitespace-nowrap text-muted-foreground hover:text-foreground",
              isActive && "border-primary font-medium text-foreground",
            )
          }
        >
          {sectionLabel}
          {section === "mail" && (
            <ApiProvider target={targetFor(hostId)}>
              <OpenMailCount />
            </ApiProvider>
          )}
        </NavLink>
      ))}
    </nav>
  );
}
