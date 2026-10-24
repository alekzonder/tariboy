import type { CodexHarnessPresentation } from "@/lib/auditCodex";

// CodexDetails renders the readable Codex activity shown above the raw event
// data when an event row is expanded.
export function CodexDetails({ codex }: { codex: CodexHarnessPresentation }) {
  return (
    <div
      className="mb-2 space-y-1"
      aria-label={codex.type === "command" ? "Command details" : "Activity details"}
    >
      <p>
        <span className="font-semibold">{codex.label}:</span> {codex.text}
      </p>
      {codex.status && (
        <p>
          <span className="font-semibold">Status:</span> {codex.status}
        </p>
      )}
      {codex.output !== undefined && (
        <pre className="overflow-x-auto whitespace-pre-wrap break-words rounded border p-2">
          <span className="font-semibold">Output:</span>
          {"\n"}
          {codex.output}
        </pre>
      )}
    </div>
  );
}
