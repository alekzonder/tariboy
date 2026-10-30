import * as React from "react";
import { Search } from "lucide-react";

import { cn } from "@/lib/utils";
import { TEXT_INPUT_ASSISTANCE } from "./input-assistance";

/**
 * A borderless search field inside a muted pill with a magnifier icon, as in
 * the agent sidebar and the task filter bar. `className` styles the pill,
 * `iconClassName` and `inputClassName` the icon and the field; every other
 * prop goes to the `<input type="search">`. `hint` is a shortcut shown after
 * the field, such as ⌘K.
 */
function SearchInput({
  className,
  iconClassName,
  inputClassName,
  hint,
  ...props
}: Omit<React.ComponentProps<"input">, "type"> & {
  iconClassName?: string;
  inputClassName?: string;
  hint?: React.ReactNode;
}) {
  return (
    <label
      data-slot="search-input"
      className={cn(
        "flex items-center gap-1.75 rounded-md bg-muted px-2.25 text-muted-foreground",
        className,
      )}
    >
      <Search aria-hidden="true" className={cn("size-3 shrink-0", iconClassName)} />
      <input
        type="search"
        {...TEXT_INPUT_ASSISTANCE}
        className={cn(
          "min-w-0 flex-1 bg-transparent text-foreground outline-none placeholder:text-muted-foreground",
          inputClassName,
        )}
        {...props}
      />
      {hint ? (
        <span aria-hidden="true" className="shrink-0 text-caption opacity-60">
          {hint}
        </span>
      ) : null}
    </label>
  );
}

export { SearchInput };
