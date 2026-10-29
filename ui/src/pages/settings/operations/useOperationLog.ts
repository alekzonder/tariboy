import { useCallback, useRef, useState } from "react";
import { useApi } from "@/components/apiContext";
import { errorText } from "@/lib/errors";

/** The bounded read loop never asks for more chunks than this in one call. */
const MAX_CHUNKS = 64;

/**
 * The log of one operation, read in chunks from the last offset. `readMore`
 * appends everything written since the previous read; the caller serializes
 * calls (see useCoalesced).
 */
export function useOperationLog(id: number) {
  const api = useApi();
  const next = useRef(0);
  const [content, setContent] = useState("");
  const [available, setAvailable] = useState(true);
  const [error, setError] = useState("");

  const readMore = useCallback(async () => {
    try {
      for (let chunk = 0; chunk < MAX_CHUNKS; chunk++) {
        const log = await api.operations.log(id, next.current);
        if (!log.available) {
          setAvailable(false);
          return;
        }
        setAvailable(true);
        if (log.content) setContent((previous) => previous + log.content);
        const advanced = log.next > next.current;
        next.current = log.next;
        if (!advanced || log.next >= log.size) break;
      }
      setError("");
    } catch (cause) {
      setError(errorText(cause));
    }
  }, [api, id]);

  return { content, available, error, readMore };
}
