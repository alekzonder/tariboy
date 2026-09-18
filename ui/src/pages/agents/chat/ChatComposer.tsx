import { useEffect, useRef, useState, type ChangeEvent } from "react";
import { Paperclip } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { ApiError, serverUploadFile, type ApiTarget } from "@/lib/api";
import ChatMarkdown from "./ChatMarkdown";

/** The glyph toolbar: mono marks rather than icons, because each one is the
 *  Markdown it inserts. `wrap` is what goes on each side of the selection;
 *  `prefix` is for the line-leading marks. */
const MARKS = [
  { label: "B", title: "Bold", wrap: "**" },
  { label: "i", title: "Italic", wrap: "*" },
  { label: "</>", title: "Inline code", wrap: "`" },
  { label: "“", title: "Quote", prefix: "> " },
  { label: "•", title: "Bullet list", prefix: "- " },
  { label: "↗", title: "Link", wrap: "", link: true },
] as const;

export default function ChatComposer({
  agent, target, value, onChange, onSend, sending, hint,
}: {
  agent: string;
  /** The host this chat belongs to: an attachment must land on the same
   *  server the agent reads from, never on whichever daemon is active. */
  target: ApiTarget;
  value: string;
  onChange: (value: string) => void;
  onSend: () => void;
  sending: boolean;
  /** What this message will do beyond being a message, when it will. */
  hint?: string;
}) {
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
      const { abs } = await serverUploadFile(picked, target);
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

  function apply(mark: typeof MARKS[number]) {
    const area = box.current;
    if (!area) return;
    const start = area.selectionStart ?? value.length;
    const end = area.selectionEnd ?? start;
    const selected = value.slice(start, end);
    let next: string;
    let caret: number;
    if ("prefix" in mark && mark.prefix) {
      const lineStart = value.lastIndexOf("\n", start - 1) + 1;
      next = `${value.slice(0, lineStart)}${mark.prefix}${value.slice(lineStart)}`;
      caret = end + mark.prefix.length;
    } else if ("link" in mark && mark.link) {
      next = `${value.slice(0, start)}[${selected || "text"}](url)${value.slice(end)}`;
      caret = start + (selected || "text").length + 3;
    } else {
      next = `${value.slice(0, start)}${mark.wrap}${selected}${mark.wrap}${value.slice(end)}`;
      caret = end + mark.wrap.length * 2;
    }
    onChange(next);
    requestAnimationFrame(() => { area.focus(); area.setSelectionRange(caret, caret); });
  }

  return (
    <div className="shrink-0 px-3.5 pt-2 pb-3">
      <div className="overflow-hidden rounded-[10px] border border-input bg-card focus-within:ring-1 focus-within:ring-ring">
        <div className="flex h-7 items-center gap-px border-b px-1.5 py-0.5" role="group" aria-label="Markdown marks">
          {MARKS.map((mark) => (
            <button
              key={mark.title}
              type="button"
              aria-label={mark.title}
              title={mark.title}
              onClick={() => apply(mark)}
              className="grid h-[22px] min-w-[22px] place-items-center rounded-[6px] px-1.5 font-mono text-[11.5px] font-semibold text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              {mark.label}
            </button>
          ))}
          <span className="mx-1.5 h-3.5 w-px bg-border" />
          <input ref={file} type="file" className="hidden" onChange={(event) => void attach(event)} />
          <button
            type="button"
            onClick={() => file.current?.click()}
            disabled={uploading}
            className="flex h-[22px] items-center gap-1.5 rounded-[6px] px-1.5 text-[11.5px] text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
          >
            <Paperclip className="size-3" />
            {uploading ? "Uploading…" : "Attach"}
          </button>
          <span className="ml-auto text-[10.5px] text-muted-foreground opacity-80">Markdown supported</span>
        </div>
        {preview ? (
          <div className="min-h-[52px] px-2.5 py-2">
            {value.trim()
              ? <ChatMarkdown onOpenTask={() => undefined}>{value}</ChatMarkdown>
              : <p className="text-[12.5px] text-muted-foreground">Nothing to preview yet.</p>}
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
            className="block w-full resize-none bg-transparent px-2.5 py-2 text-[12.5px] leading-[1.5] outline-none"
          />
        )}
        <div className="flex items-center gap-2 pr-2 pb-[7px] pl-2.5">
          {hint && <span className="text-[10.5px] text-muted-foreground">{hint}</span>}
          <div className="ml-auto flex items-center gap-1.5">
            <button
              type="button"
              aria-pressed={preview}
              onClick={() => setPreview((open) => !open)}
              className="h-[26px] rounded-[7px] px-2.5 text-[12px] text-muted-foreground hover:bg-accent hover:text-foreground"
            >
              Preview
            </button>
            <Button size="sm" onClick={onSend} disabled={sending || !value.trim()}>Send</Button>
          </div>
        </div>
      </div>
    </div>
  );
}
