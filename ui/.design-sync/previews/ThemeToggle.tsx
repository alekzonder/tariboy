import { ThemeToggle } from "tariboy-ui";

// useTheme() falls back to a default context value ({theme: "system"}), so the
// toggle renders standalone without a ThemeProvider. It is the outline icon
// button on the right of the application titlebar.
export const Default = () => <ThemeToggle />;

export const InTitlebar = () => (
  <div
    style={{
      display: "flex",
      alignItems: "center",
      justifyContent: "flex-end",
      gap: 8,
      height: 48,
      padding: "0 16px",
      borderBottom: "1px solid var(--border)",
    }}
  >
    <span style={{ fontSize: 14, color: "var(--muted-foreground)" }}>Настройки</span>
    <ThemeToggle />
  </div>
);
