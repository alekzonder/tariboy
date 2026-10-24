import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { isAndroidShell } from "@/lib/desktop";
import type { ServerForm } from "./useServerForm";

export function HttpsServerForm({
  baseURL,
  setBaseURL,
  token,
  setToken,
  editing,
  tokenStored,
  clearToken,
}: Pick<
  ServerForm,
  "baseURL" | "setBaseURL" | "token" | "setToken" | "editing" | "tokenStored" | "clearToken"
>) {
  return (
    <>
      <div className="space-y-1">
        <Label htmlFor="add-host-url">base URL</Label>
        <Input
          id="add-host-url"
          value={baseURL}
          onChange={(event) => setBaseURL(event.target.value)}
          placeholder={
            isAndroidShell() ? "http://127.0.0.1:9990 or https://host:port" : "https://host:port"
          }
          className="h-8"
        />
      </div>
      <div className="space-y-1">
        <Label htmlFor="add-host-token">token</Label>
        <Input
          id="add-host-token"
          type="password"
          value={token}
          onChange={(event) => setToken(event.target.value)}
          placeholder={editing ? "leave empty to keep the current token" : ""}
          className="h-8"
        />
        {editing && (
          <div className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground">
              {tokenStored ? "token set for this session" : "no token set"}
            </span>
            {tokenStored && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-xs"
                onClick={() => void clearToken()}
              >
                Clear token
              </Button>
            )}
          </div>
        )}
      </div>
    </>
  );
}
