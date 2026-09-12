import {
  Button,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Input,
  Label,
  Switch,
} from "tariboy-ui";
import { ChevronDown, ChevronRight } from "lucide-react";

export const AdvancedOptionsOpen = () => (
  <Collapsible defaultOpen style={{ width: 420 }}>
    <CollapsibleTrigger asChild>
      <Button variant="outline" size="sm" type="button">
        <ChevronDown className="size-4" />
        Advanced
      </Button>
    </CollapsibleTrigger>
    <CollapsibleContent style={{ marginTop: 12, display: "grid", gap: 12 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <Switch id="interactive" defaultChecked />
        <Label htmlFor="interactive">interactive (tmux TUI)</Label>
      </div>
      <div style={{ display: "grid", gap: 6 }}>
        <Label>env (K=V)</Label>
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <Input
            aria-label="env key 0"
            defaultValue="TARIBOY_PROFILE"
            style={{ height: 32, width: 168 }}
          />
          <span style={{ color: "var(--muted-foreground)" }}>=</span>
          <Input
            aria-label="env value 0"
            defaultValue="team"
            style={{ height: 32, width: 120 }}
          />
        </div>
      </div>
    </CollapsibleContent>
  </Collapsible>
);

export const AdvancedOptionsClosed = () => (
  <Collapsible style={{ width: 420 }}>
    <CollapsibleTrigger asChild>
      <Button variant="outline" size="sm" type="button">
        <ChevronRight className="size-4" />
        Advanced
      </Button>
    </CollapsibleTrigger>
    <CollapsibleContent style={{ marginTop: 12, display: "grid", gap: 12 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
        <Switch id="interactive-closed" />
        <Label htmlFor="interactive-closed">interactive (tmux TUI)</Label>
      </div>
    </CollapsibleContent>
  </Collapsible>
);

export const IterationDetail = () => (
  <Collapsible
    defaultOpen
    style={{
      width: 440,
      border: "1px solid var(--border)",
      borderRadius: 10,
      padding: 12,
    }}
  >
    <CollapsibleTrigger asChild>
      <Button variant="ghost" size="sm" type="button" style={{ paddingLeft: 4 }}>
        <ChevronDown className="size-4" />
        Iteration 42 · builder · 1m 18s
      </Button>
    </CollapsibleTrigger>
    <CollapsibleContent style={{ marginTop: 8 }}>
      <pre
        style={{
          margin: 0,
          font: "12px/18px ui-monospace, SFMono-Regular, Menlo, monospace",
          color: "var(--muted-foreground)",
          whiteSpace: "pre-wrap",
        }}
      >
        {"claimed TB-142 from queue tasks\nimage worker:v2 · host This daemon (local)\nexit 0 · $0.14"}
      </pre>
    </CollapsibleContent>
  </Collapsible>
);
