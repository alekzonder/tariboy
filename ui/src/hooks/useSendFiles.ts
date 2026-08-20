import { useCallback, useState } from "react"
import { toast } from "sonner"
import { agentUploadFile, ApiError } from "@/lib/api"
import type { Daemon } from "@/lib/daemons"

/** Upload files to an agent's `.tariboy/files` directory and report their
 * saved absolute paths to the caller.  Buttons and drop targets share this
 * path so they retain identical progress and error behaviour. */
export function useSendFiles({
  name,
  daemon,
  onUploaded,
}: {
  name: string
  daemon?: Daemon | null
  onUploaded: (paths: string[]) => void
}) {
  const [uploading, setUploading] = useState(false)

  const sendFiles = useCallback(async (files: File[]) => {
    if (files.length === 0) return
    setUploading(true)
    try {
      const paths: string[] = []
      for (const file of files) {
        const { abs } = await agentUploadFile(name, file, daemon)
        paths.push(abs)
      }
      onUploaded(paths)
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : String(err))
    } finally {
      setUploading(false)
    }
  }, [daemon, name, onUploaded])

  return { uploading, sendFiles }
}
