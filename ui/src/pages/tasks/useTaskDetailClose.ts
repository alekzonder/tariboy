import { useEffect, useRef, useState } from "react";

/**
 * Closing the panel, or opening another task in it, with the draft
 * confirmation: either one asks first while a draft would be lost.
 */
export function useTaskDetailClose(
  hasDraft: boolean,
  pending: boolean,
  onClose: () => void,
  onOpenTask?: (key: string) => void,
) {
  const [returnFocus] = useState(() => document.activeElement as HTMLElement | null);
  const initialFocusRef = useRef<HTMLButtonElement>(null);
  // The panel's title, focused when the panel moved here from another task.
  const titleRef = useRef<HTMLHeadingElement>(null);
  const [confirmClose, setConfirm] = useState(false);
  // The task the confirmation opens instead of closing the panel.
  const [openAfter, setOpenAfter] = useState("");
  const setConfirmClose = (open: boolean) => {
    setConfirm(open);
    if (!open) setOpenAfter("");
  };
  const close = () => {
    if (pending) return;
    setOpenAfter("");
    if (hasDraft) setConfirm(true);
    else onClose();
  };
  const requestOpen = (key: string) => {
    if (pending || !onOpenTask) return;
    if (!hasDraft) return onOpenTask(key);
    setOpenAfter(key);
    setConfirm(true);
  };
  /** The confirmed discard: it does what the confirmation was asked for. */
  const discard = () => (openAfter && onOpenTask ? onOpenTask(openAfter) : onClose());
  useEffect(() => {
    if (!hasDraft) return;
    const prevent = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", prevent);
    return () => window.removeEventListener("beforeunload", prevent);
  }, [hasDraft]);
  return {
    returnFocus,
    initialFocusRef,
    titleRef,
    confirmClose,
    setConfirmClose,
    close,
    requestOpen: onOpenTask ? requestOpen : undefined,
    discard,
  };
}
