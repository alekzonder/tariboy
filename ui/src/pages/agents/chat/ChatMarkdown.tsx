import { useState, type ReactNode } from "react";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import { Check, Copy } from "lucide-react";
import { isDesktop, openExternalUrl } from "@/lib/desktop";
import { toast } from "sonner";
import { TASK_KEY } from "./chatFeed";

/** The scheme a task key is rewritten to, so the link renderer can tell a task
 *  from a web address without guessing at the text. */
const TASK_HREF = "tariboy-task:";

interface MdNode {
  type: string;
  value?: string;
  url?: string;
  children?: MdNode[];
}

/**
 * Rewrite task keys written in prose into links.
 *
 * It runs as a remark plugin rather than over the raw Markdown string on
 * purpose: by the time the tree exists, code spans, fenced blocks and existing
 * link labels are their own node types, so a key inside `TB-142` or inside a
 * shell snippet is left alone without any regex having to understand Markdown.
 */
function remarkTaskKeys() {
  return (tree: MdNode) => {
    const walk = (node: MdNode) => {
      if (!node.children) return;
      const rebuilt: MdNode[] = [];
      for (const child of node.children) {
        if (child.type === "link" || child.type === "linkReference") {
          rebuilt.push(child);
          continue; // A key inside a link label is already a link.
        }
        if (child.type !== "text" || !child.value) {
          walk(child);
          rebuilt.push(child);
          continue;
        }
        let last = 0;
        TASK_KEY.lastIndex = 0;
        for (let hit = TASK_KEY.exec(child.value); hit; hit = TASK_KEY.exec(child.value)) {
          if (hit.index > last) rebuilt.push({ type: "text", value: child.value.slice(last, hit.index) });
          rebuilt.push({
            type: "link", url: `${TASK_HREF}${hit[0]}`,
            children: [{ type: "text", value: hit[0] }],
          });
          last = hit.index + hit[0].length;
        }
        if (last === 0) rebuilt.push(child);
        else if (last < child.value.length) rebuilt.push({ type: "text", value: child.value.slice(last) });
      }
      node.children = rebuilt;
    };
    walk(tree);
  };
}

function CodeBlock({ children }: { children: ReactNode }) {
  const [copied, setCopied] = useState(false);
  // react-markdown hands `pre` its `code` element; the language rides on it as
  // the `language-*` class remark put there.
  const code = (children ?? null) as { props?: { className?: string; children?: ReactNode } } | null;
  const language = /language-(\w+)/.exec(code?.props?.className ?? "")?.[1] ?? "text";
  const text = String(code?.props?.children ?? "");
  return (
    <div className="my-1 overflow-hidden rounded-[8px] bg-muted">
      <div className="flex h-[22px] items-center gap-1.5 pr-2 pl-2.5">
        <span className="font-mono text-[10px] tracking-wide text-muted-foreground">{language}</span>
        <button
          type="button"
          aria-label="Copy code"
          className="ml-auto flex h-[18px] items-center gap-1 rounded-[5px] px-1.5 text-[10.5px] text-muted-foreground hover:bg-accent hover:text-foreground"
          onClick={() => {
            void navigator.clipboard?.writeText(text.replace(/\n$/, ""))
              .then(() => { setCopied(true); setTimeout(() => setCopied(false), 1200); })
              .catch(() => toast.error("Could not copy the code block"));
          }}
        >
          {copied ? <Check className="size-3" /> : <Copy className="size-3" />}
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre className="m-0 px-2.5 pb-2 font-mono text-[11.5px] leading-[1.5] break-words whitespace-pre-wrap">
        {text.replace(/\n$/, "")}
      </pre>
    </div>
  );
}

/** Chat-density Markdown. The pipeline is the same safe one task comments use —
 *  `react-markdown` with `remark-gfm` and no raw HTML — so a message can no
 *  more execute script than a task comment can. */
export default function ChatMarkdown({ children, onOpenTask }: {
  children: string;
  onOpenTask: (key: string) => void;
}) {
  return (
    <div className="chat-markdown text-[12.5px] leading-[1.5] text-pretty [&_blockquote]:my-[3px] [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:py-0.5 [&_blockquote]:pl-[9px] [&_blockquote]:text-muted-foreground [&_code]:rounded-[5px] [&_code]:bg-muted [&_code]:px-1 [&_code]:font-mono [&_code]:text-[11.5px] [&_em]:italic [&_li]:my-0 [&_ol]:my-[2px] [&_ol]:list-decimal [&_ol]:pl-5 [&_p]:my-[1px] [&_pre_code]:bg-transparent [&_pre_code]:p-0 [&_strong]:font-semibold [&_table]:my-2 [&_table]:block [&_table]:w-full [&_table]:overflow-x-auto [&_td]:border [&_td]:px-1.5 [&_td]:py-0.5 [&_th]:border [&_th]:bg-muted [&_th]:px-1.5 [&_th]:py-0.5 [&_ul]:my-[2px] [&_ul]:list-disc [&_ul]:pl-5">
      <ReactMarkdown
        remarkPlugins={[remarkGfm, remarkTaskKeys]}
        // The default transform drops every scheme it does not know, which
        // would blank the task links this component just made. Everything else
        // keeps the library's own allowlist.
        urlTransform={(url) => (url.startsWith(TASK_HREF) ? url : defaultUrlTransform(url))}
        components={{
          pre: ({ children: content }) => <CodeBlock>{content}</CodeBlock>,
          a: ({ href, children: content }) => {
            const key = href?.startsWith(TASK_HREF) ? href.slice(TASK_HREF.length) : "";
            return (
              <a
                href={href}
                className="text-primary underline decoration-primary/35 underline-offset-2"
                onClick={(event) => {
                  if (key) { event.preventDefault(); onOpenTask(key); return; }
                  if (!isDesktop() || !href) return;
                  let url: URL;
                  try { url = new URL(href); } catch { return; }
                  if (url.protocol !== "http:" && url.protocol !== "https:") return;
                  event.preventDefault();
                  void openExternalUrl(url.toString()).catch(() => toast.error("Could not open web link"));
                }}
              >
                {content}
              </a>
            );
          },
        }}
      >
        {children}
      </ReactMarkdown>
    </div>
  );
}
