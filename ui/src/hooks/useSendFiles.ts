import { useCallback, useState } from "react"
import { toast } from "sonner"
import { serverUploadFile, ApiError } from "@/lib/api"
import type { Daemon } from "@/lib/daemons"

/** Upload files to the server's shared directory and report their absolute
 * paths. Buttons and drop targets share progress and error behaviour. */
export function useSendFiles({
  daemon,
  onUploaded,
}: {
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
        const { abs } = await serverUploadFile(file, daemon)
        paths.push(abs)
      }
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : String(err))
    } finally {
      if (paths.length > 0) onUploaded(paths)
      setUploading(false)
    }
  }, [daemon, onUploaded])

  return { uploading, sendFiles }
}
