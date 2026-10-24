import { hostInstallBlocked, INSTALL_REQUIREMENTS } from "@/lib/daemons";
import type { ServerForm } from "./useServerForm";

export function HostDiagnostics({
  displayHost,
  output,
}: Pick<ServerForm, "displayHost" | "output">) {
  const prerequisites = displayHost?.prerequisites ?? [];
  const installRequirements = prerequisites.filter((item) => INSTALL_REQUIREMENTS.has(item));
  const optionalTools = prerequisites.filter((item) => !INSTALL_REQUIREMENTS.has(item));
  const unsupported = hostInstallBlocked(displayHost?.platform, displayHost?.arch, prerequisites);

  return (
    <>
      {displayHost?.platform && displayHost.arch && (
        <p className={unsupported ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
          {unsupported
            ? `Unsupported host: ${displayHost.platform}/${displayHost.arch}`
            : `Server: ${displayHost.platform}/${displayHost.arch}`}
        </p>
      )}
      {installRequirements.length > 0 && (
        <div className="space-y-0.5 text-xs text-destructive">
          <p className="font-medium">Install blocked</p>
          <p>Missing install requirements: {installRequirements.join(", ")}</p>
        </div>
      )}
      {optionalTools.length > 0 && (
        <p className="text-xs text-muted-foreground">
          Optional agent tools not found: {optionalTools.join(", ")}
        </p>
      )}

      {output.length > 0 && (
        <details className="min-w-0 rounded-md border bg-muted/40">
          <summary className="cursor-pointer px-2 py-1.5 text-xs font-medium">
            Technical details
          </summary>
          <pre
            aria-label="SSH technical details"
            className="max-h-40 min-w-0 max-w-full overflow-auto border-t p-2 text-xs whitespace-pre-wrap [overflow-wrap:anywhere]"
          >
            {output.map((event) => event.text).join("\n")}
          </pre>
        </details>
      )}
    </>
  );
}
