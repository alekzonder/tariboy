import { useRef, useState, type ChangeEvent } from "react"
import { Paperclip } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { agentUploadFile, ApiError } from "@/lib/api"
import { useAgentName } from "@/lib/agent"

/** Pick a file, upload it under the agent's cwd at `.tariboy/files/`, and hand
 * the saved absolute host path back to the caller (which inserts it into the
 * issue description). Mirrors the "Send files" flow in TuiScreen. */
export function AttachButton({ onAttached }: { onAttached: (path: string) => void }) {
  const name = useAgentName()
  const inputRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)

  const onChange = async (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = "" // let the same file be picked again later
    if (!file) return
    setBusy(true)
    try {
      const { abs } = await agentUploadFile(name, file)
      onAttached(abs)
      toast.success(`file uploaded: ${abs}`)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <input ref={inputRef} type="file" className="hidden" onChange={(e) => void onChange(e)} />
      <Button
        type="button"
        variant="outline"
        size="sm"
        disabled={busy}
        onClick={() => inputRef.current?.click()}
      >
        <Paperclip className="size-4" />
        {busy ? "uploading…" : "Attach"}
      </Button>
    </>
  )
}
