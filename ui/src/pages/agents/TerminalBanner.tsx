import { Link } from "react-router-dom";

/** Explanation for agents configured without an interactive terminal. */
export function TerminalBanner({ settings }: { settings: string }) {
  return (
    <div role="status" className="px-1 text-sm text-muted-foreground">
      No interactive terminal. Enable it in{" "}
      <Link className="text-primary underline" to={settings}>
        Settings → Autopilot
      </Link>
      .
    </div>
  );
}
