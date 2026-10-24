import type { ButtonHTMLAttributes } from "react";
import { useDraggable, useDroppable } from "@dnd-kit/core";
import { cn } from "@/lib/utils";

/** A button that is both a drag source and a drop target for sidebar reorder. */
export function SortableButton({
  dragKey,
  disabled,
  style,
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  dragKey: string;
}) {
  const drag = useDraggable({ id: dragKey, disabled });
  const drop = useDroppable({ id: dragKey, disabled });
  return (
    <button
      ref={(node) => {
        drag.setNodeRef(node);
        drop.setNodeRef(node);
      }}
      disabled={disabled}
      style={style}
      className={cn(
        "cursor-grab active:cursor-grabbing",
        drop.isOver && "ring-1 ring-primary",
        className,
        drag.isDragging && "opacity-45",
      )}
      {...props}
      {...drag.attributes}
      {...drag.listeners}
    />
  );
}
/* eslint-enable react-hooks/refs */
