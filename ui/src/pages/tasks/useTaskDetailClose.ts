import { useEffect, useRef, useState } from "react"

export function useTaskDetailClose(hasDraft: boolean, pending: boolean, onClose: () => void) {
  const [returnFocus] = useState(() => document.activeElement as HTMLElement | null)
  const initialFocusRef = useRef<HTMLButtonElement>(null)
  const [confirmClose, setConfirmClose] = useState(false)
  const close = () => {
    if (pending) return
    if (hasDraft) setConfirmClose(true)
    else onClose()
  }
  useEffect(() => {
    if (!hasDraft) return
    const prevent = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = "" }
    window.addEventListener("beforeunload", prevent)
    return () => window.removeEventListener("beforeunload", prevent)
  }, [hasDraft])
  return { returnFocus, initialFocusRef, confirmClose, setConfirmClose, close }
}
