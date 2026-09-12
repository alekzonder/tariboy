import {
  Button,
  DropdownMenu,
  DropdownMenuCheckItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "tariboy-ui";
import {
  Copy,
  Download,
  MoonIcon,
  Pencil,
  SunIcon,
  Trash2,
  Upload,
} from "lucide-react";

// The sidebar host row the "manage host" menu hangs off.
const HostRow = ({ children }: { children?: React.ReactNode }) => (
  <div
    style={{
      width: 280,
      display: "flex",
      alignItems: "center",
      gap: 8,
      padding: "4px 8px",
      borderRadius: 6,
      border: "1px solid var(--border)",
    }}
  >
    <span style={{ fontSize: 13, fontWeight: 500 }}>build-01</span>
    <span style={{ fontSize: 12, color: "var(--muted-foreground)" }}>2 agents</span>
    <span style={{ marginLeft: "auto", display: "flex", gap: 4 }}>{children}</span>
  </div>
);

export const HostActions = () => (
  <HostRow>
    <DropdownMenu defaultOpen>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" aria-label="manage build-01">
          ⋯
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem>Edit host</DropdownMenuItem>
        <DropdownMenuItem>Remove host</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  </HostRow>
);

export const CheckedItems = () => (
  <div style={{ display: "flex", alignItems: "center", gap: 8, width: 280 }}>
    <span style={{ fontSize: 13, color: "var(--muted-foreground)" }}>Appearance</span>
    <span style={{ marginLeft: "auto" }}>
      <DropdownMenu defaultOpen>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="icon-sm" aria-label="Toggle theme" title="Theme">
            <SunIcon />
            <MoonIcon style={{ display: "none" }} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          <DropdownMenuCheckItem checked>System</DropdownMenuCheckItem>
          <DropdownMenuCheckItem>Light</DropdownMenuCheckItem>
          <DropdownMenuCheckItem>Dark</DropdownMenuCheckItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </span>
  </div>
);

export const LabelledSections = () => (
  <div style={{ width: 280 }}>
    <DropdownMenu defaultOpen>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" size="sm">
          Image actions
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuLabel>worker:v2</DropdownMenuLabel>
        <DropdownMenuItem>
          <Copy />
          Clone into a new agent
        </DropdownMenuItem>
        <DropdownMenuItem>
          <Pencil />
          Edit manifest
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuLabel>Transfer</DropdownMenuLabel>
        <DropdownMenuItem>
          <Download />
          Export archive
        </DropdownMenuItem>
        <DropdownMenuItem>
          <Upload />
          Send to build-01
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem>
          <Trash2 />
          Remove image
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  </div>
);

export const DisabledItems = () => (
  <div style={{ width: 280 }}>
    <DropdownMenu defaultOpen>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" aria-label="manage build-02">
          ⋯
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start">
        <DropdownMenuLabel>build-02 is offline</DropdownMenuLabel>
        <DropdownMenuItem>Edit host</DropdownMenuItem>
        <DropdownMenuItem disabled>New agent</DropdownMenuItem>
        <DropdownMenuItem disabled>Send files</DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem>Remove host</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  </div>
);
