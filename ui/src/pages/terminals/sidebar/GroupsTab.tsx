import { Button } from "@/components/ui/button";
import {
  SECTION_TITLE_CLASS, SectionEmpty, SidebarSectionHeader,
} from "./SidebarSectionHeader";
import { SidebarAgentRow, type AgentRowActions } from "./SidebarAgentRow";
import type { SidebarAgent } from "./sidebarModel";

/**
 * Agents by group, across every server — a group is a unit of work, not a
 * property of the machine it happens to run on, so servers do not divide it.
 */
export function GroupsTab({ sections, actions, onOpenGroup, onCreate }: {
  sections: Array<{ name: string; agents: SidebarAgent[] }>;
  actions: AgentRowActions;
  onOpenGroup: (hostId: string, group: string) => void;
  onCreate: (hostId: string) => void;
}) {
  if (sections.length === 0) {
    return <div className="px-2 pt-1.5 text-[12.5px] text-muted-foreground">No groups.</div>;
  }
  return (
    <>
      {sections.map((section) => {
        // A group opens on the server its first member runs on; a group that is
        // declared but empty has only the local daemon to open.
        const hostId = section.agents[0]?.hostId ?? "";
        return (
          <section key={section.name}>
            <SidebarSectionHeader
              title={
                <Button
                  variant="ghost"
                  className={`${SECTION_TITLE_CLASS} h-auto px-0 py-0 hover:bg-transparent`}
                  aria-label={`Open team ${section.name}`}
                  title={section.name}
                  onClick={() => onOpenGroup(hostId, section.name)}
                >
                  {section.name}
                </Button>
              }
              onAdd={() => onCreate(hostId)}
              addLabel={`new agent in ${section.name}`}
            />
            {section.agents.length === 0
              ? <SectionEmpty />
              : section.agents.map((row) => (
                <SidebarAgentRow key={row.key} row={row} actions={actions} />
              ))}
          </section>
        );
      })}
    </>
  );
}
