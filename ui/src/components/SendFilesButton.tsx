import { useRef, type ChangeEvent } from "react"
import { FileUp } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useSendFiles } from "@/hooks/useSendFiles"
import type { Daemon } from "@/lib/daemons"

/** Pick one or more files, upload each under the agent's cwd at
 * `.tariboy/files/`, and hand the saved absolute host paths to the caller.
 * The caller decides where the paths go — injected into the live terminal
 * (interactive) or appended to the inbox message (non-interactive).
 * `daemon` targets a specific host (undefined = active daemon, null =
 * same-origin), for cross-host views like /terminals. */
export function SendFilesButton({
  name,
  onUploaded,
  className,
  daemon,
}: {
  name: string
  onUploaded: (paths: string[]) => void
  className?: string
  daemon?: Daemon | null
}) {
  const inputRef = useRef<HTMLInputElement>(null)
  const { uploading, sendFiles } = useSendFiles({ name, daemon, onUploaded })

  const onPick = async (e: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(e.target.files ?? [])
    e.target.value = "" // let the same file be picked again later
    if (files.length === 0) return
    await sendFiles(files)
  }

  return (
    <>
      <input
        ref={inputRef}
        type="file"
        multiple
        className="hidden"
        onChange={(e) => void onPick(e)}
      />
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={className}
        disabled={uploading}
        onClick={() => inputRef.current?.click()}
      >
        <FileUp className="size-4" />
        {uploading ? "Uploading…" : "Send files"}
      </Button>
    </>
  )
}
