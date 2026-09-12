import { SupportBundle } from "tariboy-ui";

// SupportBundle takes no props. It reads `isDesktop()` on mount and only then
// lists hosts from the Tauri side; in a browser preview there is no desktop
// shell, so the component renders its full authored card — host select seeded
// with the local host, the two disclosure blocks, the sensitive-data opt-in —
// with every control disabled and the reason stated at the bottom. That is the
// exact state the web build ships, not a degraded stand-in.
export const SettingsPanel = () => (
  <div style={{ maxWidth: 660 }}>
    <SupportBundle />
  </div>
);
