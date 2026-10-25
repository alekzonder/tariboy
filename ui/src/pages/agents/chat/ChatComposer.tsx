import { useEffect, useRef, useState, type ChangeEvent } from "react";
import { Paperclip } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ApiError } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import ChatMarkdown from "./ChatMarkdown";
import { ChatComposerMarks } from "./ChatComposerMarks";

export default function ChatComposer({
  agent,
  value,
  onChange,
  onSend,
  sending,
  hint,
}: {
  agent: string;
  value: string;
  onChange: (value: string) => void;
  onSend: () => void;
  sending: boolean;
  /** What this message will do beyond being a message, when it will. */
  hint?: string;
}) {
  // The host this chat belongs to: an attachment must land on the same server
  // the agent reads from, never on whichever daemon is active.
  const api = useApi();
  const box = useRef<HTMLTextAreaElement | null>(null);
  const file = useRef<HTMLInputElement | null>(null);
  const [preview, setPreview] = useState(false);
  const [uploading, setUploading] = useState(false);

  /* Attach is the same upload the rest of the app does: the file goes to the
     selected server's shared directory and its absolute host path is written
     into the draft, because the agent reads a path, not a browser blob. */
  async function attach(event: ChangeEvent<HTMLInputElement>) {
    const picked = event.target.files?.[0];
    event.target.value = "";
    if (!picked) return;
    setUploading(true);
    try {
      const { abs } = await api.files.serverUploadFile(picked);
      onChange(value ? `${value}\n${abs}` : abs);
    } catch (error) {
      toast.error(error instanceof ApiError ? error.message : String(error));
    } finally {
      setUploading(false);
    }
  }

  // Two rows, growing to six. Reset first so deleting text shrinks it again.
  useEffect(() => {
    const area = box.current;
    if (!area) return;
    area.style.height = "auto";
    area.style.height = `${Math.min(area.scrollHeight, 6 * 19 + 16)}px`;
  }, [value]);

  return (
    <div className="shrink-0 px-3.5 pt-2 pb-3">
      <div className="overflow-hidden rounded-lg border border-input bg-card focus-within:ring-1 focus-within:ring-ring">
        <div
          className="flex h-7 items-center gap-px border-b px-1.5 py-0.5"
          role="group"
          aria-label="Markdown marks"
        >
          <ChatComposerMarks box={box} value={value} onChange={onChange} />
          <span className="mx-1.5 h-3.5 w-px bg-border" />
          <input
            ref={file}
            type="file"
            className="hidden"
            onChange={(event) => void attach(event)}
          />
          <button
            type="button"
            onClick={() => file.current?.click()}
            disabled={uploading}
            className="flex h-5.5 items-center gap-1.5 rounded-sm px-1.5 text-meta text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
          >
            <Paperclip className="size-3" />
            {uploading ? "Uploading…" : "Attach"}
          </button>
          <span className="ml-auto text-tiny text-muted-foreground opacity-80">
            Markdown supported
          </span>
        </div>
        {preview ? (
          <div className="min-h-13 px-2.5 py-2">
            {value.trim() ? (
              <ChatMarkdown onOpenTask={() => undefined}>{value}</ChatMarkdown>
            ) : (
              <p className="text-body-sm text-muted-foreground">Nothing to preview yet.</p>
            )}
          </div>
        ) : (
          <textarea
            ref={box}
            rows={2}
            aria-label={`Message ${agent}`}
            placeholder={`Message ${agent}…  ⌘↵ to send`}
            value={value}
            onChange={(event) => onChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
                event.preventDefault();
                onSend();
              }
              if (event.key === "Escape") event.currentTarget.blur();
            }}
            className="block w-full resize-none bg-transparent px-2.5 py-2 text-body-sm leading-[1.5] outline-none"
          />
        )}
        <div className="flex items-center gap-2 pr-2 pb-1.75 pl-2.5">
          {hint && <span className="text-tiny text-muted-foreground">{hint}</span>}
          <div className="ml-auto flex items-center gap-1.5">
            <button
              type="button"
              aria-pressed={preview}
              onClick={() => setPreview((open) => !open)}
              className="h-6.5 rounded-control px-2.5 text-ui text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              Preview
            </button>
            <Button size="sm" onClick={onSend} disabled={sending || !value.trim()}>
              Send
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
