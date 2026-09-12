import { useEffect, useRef, useState } from "react";
import { Label, PathAutocomplete } from "tariboy-ui";

// PathAutocomplete is controlled: the caller owns the path string (New agent →
// cwd, the Configuration tab's working directory, the terminals Create-agent
// dialog). The dropdown's folder listing comes from the daemon's /api/fs/list,
// so with no daemon the list is the "nothing to offer for this prefix" branch —
// the input itself, its placeholder and the popover chrome are all real.

const Field = ({ children }: { children: React.ReactNode }) => (
  <div style={{ width: 460, display: "flex", flexDirection: "column", gap: 6 }}>
    <Label htmlFor="cwd">cwd</Label>
    {children}
    {/* Room for the absolutely-positioned dropdown, which overlays the form
        row in the product rather than pushing it down. */}
    <div style={{ height: 140 }} />
  </div>
);

// The resting state in the New agent form: a typed path, list closed.
export const WorkingDirectory = () => {
  const [value, setValue] = useState("/home/agent/github/tariboy");
  return (
    <Field>
      <PathAutocomplete
        id="cwd"
        value={value}
        onChange={setValue}
        placeholder="empty = managed workdir"
        aria-label="cwd"
      />
    </Field>
  );
};

// Empty means "managed workdir" — the placeholder carries that rule.
export const ManagedWorkdir = () => {
  const [value, setValue] = useState("");
  return (
    <Field>
      <PathAutocomplete
        id="cwd"
        value={value}
        onChange={setValue}
        placeholder="empty = managed workdir"
        aria-label="cwd"
      />
    </Field>
  );
};

// Focused, so the component's own dropdown is open. The listing request is made
// for real and fails (no daemon behind the preview), which is exactly the path
// that renders "No matching folders" — the same empty the operator sees after
// typing a prefix outside the filesystem root.
export const OpenListing = () => {
  const [value, setValue] = useState("/home/agent/github/tariboy/ui/sr");
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let tries = 0;
    const id = window.setInterval(() => {
      const input = ref.current?.querySelector("input") as HTMLInputElement | null;
      if (!input) { if (++tries > 40) window.clearInterval(id); return; }
      window.clearInterval(id);
      input.focus();
    }, 25);
    return () => window.clearInterval(id);
  }, []);
  return (
    <Field>
      <div ref={ref}>
        <PathAutocomplete
          id="cwd"
          value={value}
          onChange={setValue}
          placeholder="empty = managed workdir"
          aria-label="cwd"
          debounceMs={0}
        />
      </div>
    </Field>
  );
};
