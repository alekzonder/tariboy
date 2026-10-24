import { useCallback, useState } from "react";
import { toast } from "sonner";
import { ApiError } from "@/lib/api";
import { useApi } from "@/components/apiContext";

/** Upload files to the server's shared directory and report their absolute
 * paths. Buttons and drop targets share progress and error behaviour. The
 * server is the surrounding ApiProvider's. */
export function useSendFiles({ onUploaded }: { onUploaded: (paths: string[]) => void }) {
  const api = useApi();
  const [uploading, setUploading] = useState(false);

  const sendFiles = useCallback(
    async (files: File[]) => {
      if (files.length === 0) return;
      setUploading(true);
      const paths: string[] = [];
      try {
        for (const file of files) {
          const { abs } = await api.files.serverUploadFile(file);
          paths.push(abs);
        }
      } catch (err) {
        toast.error(err instanceof ApiError ? err.message : String(err));
      } finally {
        if (paths.length > 0) onUploaded(paths);
        setUploading(false);
      }
    },
    [api, onUploaded],
  );

  return { uploading, sendFiles };
}
