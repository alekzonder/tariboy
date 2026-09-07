import { useCallback, useState } from "react"
import { toast } from "sonner"
import { agentUploadFile, serverUploadFile, ApiError } from "@/lib/api"
import type { Daemon } from "@/lib/daemons"

/** Upload files to an agent's `.tariboy/files` directory, or to the server
 * when no agent name is supplied, and report their
 * saved absolute paths to the caller.  Buttons and drop targets share this
 * path so they retain identical progress and error behaviour. */
export function useSendFiles({
  name,
  daemon,
  onUploaded,
}: {
  name?: string
  daemon?: Daemon | null
  onUploaded: (paths: string[]) => void
}) {
  const [uploading, setUploading] = useState(false)

  const sendFiles = useCallback(async (files: File[]) => {
    if (files.length === 0) return
    setUploading(true)
    const paths: string[] = []
    try {
      for (const file of files) {
        const { abs } = await (name === undefined ? serverUploadFile(file, daemon) : agentUploadFile(name, file, daemon))
        paths.push(abs)
      }
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : String(err))
    } finally {
      if (paths.length > 0) onUploaded(paths)
      setUploading(false)
    }
  }, [daemon, name, onUploaded])

  return { uploading, sendFiles }
}
