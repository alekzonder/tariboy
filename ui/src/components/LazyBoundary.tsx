import { Component, Suspense, type ReactNode } from "react";
import { Button } from "@/components/ui/button";

/**
 * The boundary around a lazily loaded screen or editor. Suspense shows a
 * loading status in the content area while the chunk downloads; the
 * surrounding titlebar and sidebars stay mounted. A chunk that fails to load
 * (an updated or unreachable WebView origin) shows an error with an explicit
 * reload, never an automatic retry loop. A new resetKey, such as the route
 * path, clears that error.
 */
export function LazyBoundary({ resetKey, reload, children }: {
  resetKey: string;
  reload?: () => void;
  children: ReactNode;
}) {
  return (
    <LoadErrorBoundary resetKey={resetKey} reload={reload ?? (() => window.location.reload())}>
      <Suspense fallback={<LazyLoading />}>{children}</Suspense>
    </LoadErrorBoundary>
  );
}

function LazyLoading() {
  return (
    <div role="status" className="p-4 text-sm text-muted-foreground">
      Loading…
    </div>
  );
}

type BoundaryProps = { resetKey: string; reload: () => void; children: ReactNode };

class LoadErrorBoundary extends Component<BoundaryProps, { failed: boolean }> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidUpdate(previous: BoundaryProps) {
    if (this.state.failed && previous.resetKey !== this.props.resetKey) this.setState({ failed: false });
  }

  render() {
    if (!this.state.failed) return this.props.children;
    return (
      <div role="alert" className="flex flex-col items-start gap-2 p-4 text-sm">
        <p>This screen failed to load.</p>
        <p className="text-muted-foreground">Reloading discards unsaved input.</p>
        <Button type="button" size="sm" variant="outline" onClick={this.props.reload}>
          Reload
        </Button>
      </div>
    );
  }
}
