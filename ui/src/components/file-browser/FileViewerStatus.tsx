import { Loader2 } from "lucide-react";

// Placeholder shown instead of a file body: nothing selected, loading, or a
// file whose content is not displayed.
export function FileViewerStatus({
  status,
  size,
}: {
  status: "empty" | "loading" | "binary" | "too_large";
  size?: number;
}) {
  if (status === "loading") {
    return (
      <div className="flex h-full items-center justify-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" /> Loading…
      </div>
    );
  }
  const message =
    status === "empty"
      ? "Select a file to view"
      : status === "binary"
        ? `Binary file — not shown (${size} bytes)`
        : `File too large to display (${size} bytes)`;
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      {message}
    </div>
  );
}
