import { useState } from "react";
import { useApi } from "@/components/apiContext";
import { guard } from "@/lib/toast-guard";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export default function OpsPage() {
  const api = useApi();
  const [target, setTarget] = useState("all");
  const [file, setFile] = useState("");

  return (
    <div className="space-y-4 p-6">
      <h1 className="text-lg font-semibold">Ops</h1>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Backup</CardTitle>
        </CardHeader>
        <CardContent className="flex items-end gap-2">
          <Input
            value={target}
            onChange={(e) => setTarget(e.target.value)}
            placeholder="agent name or 'all'"
            className="h-8 w-56"
          />
          <Button
            size="sm"
            onClick={() =>
              void guard("backup", () => api.post("/api/backup", { target }), { verb: "ok" })
            }
          >
            Back up
          </Button>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-base">Restore</CardTitle>
        </CardHeader>
        <CardContent className="flex items-end gap-2">
          <Input
            value={file}
            onChange={(e) => setFile(e.target.value)}
            placeholder="daemon-accessible archive path"
            className="h-8 w-96 min-w-0"
          />
          <Button
            size="sm"
            onClick={() =>
              void guard("restore", () => api.post("/api/restore", { file }), { verb: "ok" })
            }
          >
            Restore
          </Button>
        </CardContent>
      </Card>
      <p className="text-xs text-muted-foreground">
        Paths are resolved on the daemon host; backups land under the daemon base dir's backups/ by
        default.
      </p>
    </div>
  );
}
