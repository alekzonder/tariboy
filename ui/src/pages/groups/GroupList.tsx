import type { GroupRow } from "@/lib/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

interface Props {
  groups: GroupRow[];
  onInspect: (name: string) => void;
}

export function GroupList({ groups, onInspect }: Props) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Groups</CardTitle>
      </CardHeader>
      <CardContent className="space-y-1">
        {groups.map((g) => (
          <button
            key={g.name}
            onClick={() => onInspect(g.name)}
            className="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-sm hover:bg-accent"
          >
            <span className="font-mono">{g.name}</span>
            <span className="flex items-center gap-2 text-xs text-muted-foreground">
              {g.lead && <Badge variant="secondary">lead {g.lead}</Badge>}
              <span>{g.members} members</span>
            </span>
          </button>
        ))}
        {groups.length === 0 && <p className="text-sm text-muted-foreground">No groups.</p>}
      </CardContent>
    </Card>
  );
}
