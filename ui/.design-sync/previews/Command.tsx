import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "tariboy-ui";
import { Folder, Box, Bot, Play, Square } from "lucide-react";

export const PathAutocomplete = () => (
  <Command
    shouldFilter={false}
    value="0"
    className="relative overflow-visible bg-transparent"
    style={{ width: 360 }}
  >
    <CommandInput
      id="agent-cwd"
      value="/srv/tariboy/work/bui"
      placeholder="/srv/tariboy/work"
      onValueChange={() => {}}
    />
    <CommandList
      className="mt-1 w-full rounded-lg border bg-popover shadow-md"
      style={{ position: "static" }}
    >
      {["builder", "build-cache", "buildkit-state"].map((name, i) => (
        <CommandItem key={name} value={String(i)}>
          <Folder className="size-4" />
          {name}
        </CommandItem>
      ))}
    </CommandList>
  </Command>
);

export const AgentPalette = () => (
  <Command
    value="builder"
    className="rounded-lg border shadow-md"
    style={{ width: 380 }}
  >
    <CommandInput placeholder="Search agents and actions…" />
    <CommandList>
      <CommandGroup heading="Agents">
        <CommandItem value="builder">
          <Bot className="size-4" />
          builder
          <span style={{ marginLeft: "auto", color: "var(--muted-foreground)", fontSize: 12 }}>
            worker:v2
          </span>
        </CommandItem>
        <CommandItem value="reviewer">
          <Bot className="size-4" />
          reviewer
          <span style={{ marginLeft: "auto", color: "var(--muted-foreground)", fontSize: 12 }}>
            reviewer:v3
          </span>
        </CommandItem>
        <CommandItem value="packager">
          <Bot className="size-4" />
          packager
          <span style={{ marginLeft: "auto", color: "var(--muted-foreground)", fontSize: 12 }}>
            bare:latest
          </span>
        </CommandItem>
      </CommandGroup>
      <CommandSeparator />
      <CommandGroup heading="Actions">
        <CommandItem value="start-loop">
          <Play className="size-4" />
          Start loop on builder
        </CommandItem>
        <CommandItem value="stop-loop">
          <Square className="size-4" />
          Stop loop on builder
        </CommandItem>
      </CommandGroup>
    </CommandList>
  </Command>
);

export const NoMatchingFolders = () => (
  <Command
    shouldFilter={false}
    className="relative overflow-visible bg-transparent"
    style={{ width: 360 }}
  >
    <CommandInput
      id="agent-cwd-empty"
      value="/srv/tariboy/work/zzz"
      onValueChange={() => {}}
    />
    <CommandList
      className="mt-1 w-full rounded-lg border bg-popover shadow-md"
      style={{ position: "static" }}
    >
      <CommandEmpty>No matching folders</CommandEmpty>
    </CommandList>
  </Command>
);

export const ImagePicker = () => (
  <Command value="worker:v2" className="rounded-lg border shadow-md" style={{ width: 340 }}>
    <CommandInput placeholder="Filter images…" />
    <CommandList>
      <CommandGroup heading="This daemon (local)">
        {["worker:v1", "worker:v2", "bare:latest"].map((ref) => (
          <CommandItem key={ref} value={ref}>
            <Box className="size-4" />
            {ref}
          </CommandItem>
        ))}
      </CommandGroup>
      <CommandSeparator />
      <CommandGroup heading="build-01">
        <CommandItem value="reviewer:v3">
          <Box className="size-4" />
          reviewer:v3
        </CommandItem>
      </CommandGroup>
    </CommandList>
  </Command>
);
