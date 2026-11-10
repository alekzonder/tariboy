import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { errorText } from "@/lib/errors";
import { metricsErrorKind } from "@/lib/metricsFormat";

/** The first load: card and table outlines, announced once. */
export function MetricsLoading() {
  return (
    <div className="space-y-3">
      <p role="status" className="text-body text-muted-foreground">
        Loading metrics…
      </p>
      <div aria-hidden="true" className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4">
        {Array.from({ length: 8 }, (_, i) => (
          <div key={i} className="h-18 animate-pulse rounded-chip border bg-subtle" />
        ))}
      </div>
      <div aria-hidden="true" className="h-40 animate-pulse rounded-chip border bg-subtle" />
    </div>
  );
}

/** A failed load: an older host, a timeout, or the error's text, with Retry.
 *  `title` names what failed to load. */
export function MetricsError({
  error,
  onRetry,
  title = "Could not load metrics.",
}: {
  error: unknown;
  onRetry: () => void;
  title?: string;
}) {
  const kind = metricsErrorKind(error);
  if (kind === "old_daemon") {
    return (
      <div role="alert" className="rounded-chip border p-3 text-body">
        <p>This host runs an older daemon without metrics. Update the host to see them.</p>
        <Link to="/settings/hosts" className="text-primary hover:underline">
          Settings → Hosts
        </Link>
      </div>
    );
  }
  return (
    <div
      role="alert"
      className="space-y-2 rounded-chip border border-destructive-line p-3 text-body"
    >
      <p className="font-medium">{title}</p>
      <p className="text-muted-foreground">
        {kind === "timeout" ? "The host did not answer in 30 seconds." : errorText(error)}
      </p>
      <Button type="button" size="sm" variant="outline" onClick={onRetry}>
        Retry
      </Button>
    </div>
  );
}
