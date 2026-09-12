import {
  Button,
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "tariboy-ui";
import { CircleHelp, Trash2 } from "lucide-react";

export const GoalHelp = () => (
  <TooltipProvider>
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 4,
        fontSize: 14,
        paddingTop: 56,
      }}
    >
      <span style={{ color: "var(--muted-foreground)" }}>Goal:</span>
      <span style={{ fontFamily: "ui-monospace, monospace", color: "var(--primary)" }}>
        TB-142
      </span>
      <Tooltip defaultOpen>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            aria-label="What is the current goal?"
            style={{ height: 24, width: 24 }}
          >
            <CircleHelp className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top" sideOffset={6}>
          The task the agent works toward each iteration until it is closed.
        </TooltipContent>
      </Tooltip>
    </div>
  </TooltipProvider>
);

export const DestructiveAction = () => (
  <TooltipProvider>
    <div style={{ paddingBottom: 56 }}>
      <Tooltip defaultOpen>
        <TooltipTrigger asChild>
          <Button variant="outline" size="sm" aria-label="Remove image">
            <Trash2 className="size-4" />
            Remove
          </Button>
        </TooltipTrigger>
        <TooltipContent side="bottom" sideOffset={6}>
          Remove worker:v2 from This daemon (local)
        </TooltipContent>
      </Tooltip>
    </div>
  </TooltipProvider>
);

export const TruncatedDigest = () => (
  <TooltipProvider>
    <div style={{ display: "flex", justifyContent: "flex-start", width: 420 }}>
      <Tooltip defaultOpen>
        <TooltipTrigger asChild>
          <span
            style={{
              fontFamily: "ui-monospace, monospace",
              fontSize: 13,
              borderBottom: "1px dotted var(--border)",
              cursor: "default",
            }}
          >
            sha256:9ac71e3f52…
          </span>
        </TooltipTrigger>
        <TooltipContent side="right" sideOffset={8} style={{ wordBreak: "break-all" }}>
          sha256:9ac71e3f5286b1d0a4ef77c3b9042de1c8a5f31b
        </TooltipContent>
      </Tooltip>
    </div>
  </TooltipProvider>
);

export const DisabledControl = () => (
  <TooltipProvider>
    <div style={{ paddingTop: 56 }}>
      <Tooltip defaultOpen>
        <TooltipTrigger asChild>
          <span style={{ display: "inline-block" }}>
            <Button variant="outline" size="sm" disabled style={{ pointerEvents: "none" }}>
              Restart builder
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent side="top" sideOffset={6}>
          build-01 is unavailable — actions resume when it reconnects.
        </TooltipContent>
      </Tooltip>
    </div>
  </TooltipProvider>
);
