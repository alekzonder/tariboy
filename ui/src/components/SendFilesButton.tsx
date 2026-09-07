import { useEffect, useRef, type ChangeEvent } from "react"
import { FileUp } from "lucide-react"
import { Button } from "@/components/ui/button"
import { useSendFiles } from "@/hooks/useSendFiles"
import type { Daemon } from "@/lib/daemons"

/** Pick files and hand their saved absolute host paths to the caller.
 * Uploads go to the server's shared files directory.
 * The caller decides where the paths go — injected into the live terminal
 * (interactive) or appended to the inbox message (non-interactive).
 * `daemon` targets a specific host (undefined = active daemon, null =
 * same-origin), for cross-host views like /terminals. */
export function SendFilesButton({
  onUploaded,
  className,
  daemon,
  disabled,
  onUploadingChange,
}: {
  onUploaded: (paths: string[]) => void
  className?: string
  daemon?: Daemon | null
  disabled?: boolean
  onUploadingChange?: (uploading: boolean) => void
}) {
  const inputRef = useRef<HTMLInputElement>(null)
  const { uploading, sendFiles } = useSendFiles({ daemon, onUploaded })
  useEffect(() => { onUploadingChange?.(uploading) }, [uploading, onUploadingChange])

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
        disabled={disabled || uploading}
        onChange={(e) => void onPick(e)}
      />
      <Button
        type="button"
        variant="outline"
        size="sm"
        className={className}
        disabled={disabled || uploading}
        onClick={() => inputRef.current?.click()}
      >
        <FileUp className="size-4" />
        {uploading ? "Uploading…" : "Send files"}
      </Button>
    </>
  )
}
