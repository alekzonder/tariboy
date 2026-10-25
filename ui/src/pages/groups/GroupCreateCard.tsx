import type { ApiClient } from "@/lib/api";
import { guard } from "@/lib/toast-guard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

interface Props {
  api: ApiClient;
  name: string;
  onNameChange: (name: string) => void;
  lead: string;
  onLeadChange: (lead: string) => void;
  onCreated: () => void;
}

export function GroupCreateCard({ api, name, onNameChange, lead, onLeadChange, onCreated }: Props) {
  const create = () =>
    guard("create", () => api.request("POST", "/api/groups", { name, lead: lead || undefined }), {
      verb: "ok",
      after: () => {
        onNameChange("");
        onLeadChange("");
        onCreated();
      },
    });
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">Create group</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-wrap items-end gap-2">
        <Input
          value={name}
          onChange={(e) => onNameChange(e.target.value)}
          placeholder="group name"
          className="h-8 w-48"
        />
        <Input
          value={lead}
          onChange={(e) => onLeadChange(e.target.value)}
          placeholder="lead (optional)"
          className="h-8 w-48"
        />
        <Button size="sm" onClick={create}>
          Create
        </Button>
      </CardContent>
    </Card>
  );
}
