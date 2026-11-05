import type { Meta, StoryObj } from "@storybook/react-vite";
import { Bot, Box, Folder, Play, Square } from "lucide-react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from "./command";

const meta = { component: Command } satisfies Meta<typeof Command>;
export default meta;

type Story = StoryObj;

const noop = () => {};
const listClass = "static mt-1 w-full rounded-lg border bg-popover shadow-md";

export const PathAutocomplete: Story = {
  render: () => (
    <Command
      shouldFilter={false}
      value="0"
      className="relative w-[360px] overflow-visible bg-transparent"
    >
      <CommandInput
        id="agent-cwd"
        value="/srv/tariboy/work/bui"
        placeholder="/srv/tariboy/work"
        onValueChange={noop}
      />
      <CommandList className={listClass}>
        {["builder", "build-cache", "buildkit-state"].map((name, i) => (
          <CommandItem key={name} value={String(i)}>
            <Folder className="size-4" />
            {name}
          </CommandItem>
        ))}
      </CommandList>
    </Command>
  ),
};

const agents = [
  { name: "builder", image: "worker:v2" },
  { name: "reviewer", image: "reviewer:v3" },
  { name: "packager", image: "bare:latest" },
];

export const AgentPalette: Story = {
  render: () => (
    <Command value="builder" className="w-[380px] rounded-lg border shadow-md">
      <CommandInput placeholder="Search agents and actions…" />
      <CommandList>
        <CommandGroup heading="Agents">
          {agents.map((agent) => (
            <CommandItem key={agent.name} value={agent.name}>
              <Bot className="size-4" />
              {agent.name}
              <span className="ml-auto text-xs text-muted-foreground">{agent.image}</span>
            </CommandItem>
          ))}
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
  ),
};

export const NoMatchingFolders: Story = {
  render: () => (
    <Command shouldFilter={false} className="relative w-[360px] overflow-visible bg-transparent">
      <CommandInput id="agent-cwd-empty" value="/srv/tariboy/work/zzz" onValueChange={noop} />
      <CommandList className={listClass}>
        <CommandEmpty>No matching folders</CommandEmpty>
      </CommandList>
    </Command>
  ),
};

export const ImagePicker: Story = {
  render: () => (
    <Command value="worker:v2" className="w-[340px] rounded-lg border shadow-md">
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
  ),
};
