import { MoreHorizontal } from "lucide-react";
import {
  DropdownMenu,
  DropdownMenuCheckItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { DEFAULT_CHAT_TYPES, EXTRA_CHAT_TYPES } from "./chatTypes";

const ALL_TYPES = [...DEFAULT_CHAT_TYPES, ...EXTRA_CHAT_TYPES];

/** The chat menu: which message types the feed shows. */
export function ChatTypesMenu({
  types,
  toggleType,
  resetTypes,
}: {
  types: string[];
  toggleType: (type: string) => void;
  resetTypes: () => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="Chat menu"
        className="grid size-6.5 place-items-center rounded-control text-muted-foreground hover:bg-accent hover:text-foreground"
      >
        <MoreHorizontal className="size-3.5" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-56">
        <DropdownMenuLabel>Message types</DropdownMenuLabel>
        {ALL_TYPES.map((type) => (
          <DropdownMenuCheckItem
            key={type}
            checked={types.includes(type)}
            onSelect={(event: Event) => {
              event.preventDefault();
              toggleType(type);
            }}
            className="font-mono text-xs"
          >
            {type}
          </DropdownMenuCheckItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={resetTypes}>Reset to the default types</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
