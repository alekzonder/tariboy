import type { MouseEvent } from "react";
import { opensWebLinks, openExternalUrl } from "@/lib/desktop";
import { toast } from "sonner";

export const markdownStyles = [
  "task-markdown max-w-none text-body-sm leading-[1.55] text-pretty [&_p]:my-2 [&_p:first-child]:mt-0 [&_p:last-child]:mb-0",
  "[&_:is(h1,h2,h3,h4,h5,h6)]:mt-4 [&_:is(h1,h2,h3,h4,h5,h6)]:mb-2 [&_:is(h1,h2,h3,h4,h5,h6)]:font-semibold [&_h1]:text-2xl [&_h2]:text-xl [&_h3]:text-lg",
  "[&_strong]:font-bold [&_em]:italic [&_del]:line-through [&_s]:line-through [&_a]:text-primary [&_a]:underline",
  "[&_ul]:my-2 [&_ul]:list-disc [&_ul]:pl-6 [&_ol]:my-2 [&_ol]:list-decimal [&_ol]:pl-6 [&_li]:my-1",
  "[&_blockquote]:my-3 [&_blockquote]:border-l-2 [&_blockquote]:border-border [&_blockquote]:pl-4 [&_blockquote]:text-muted-foreground",
  "[&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:font-mono [&_pre]:my-3 [&_pre]:overflow-x-auto [&_pre]:rounded-md [&_pre]:bg-muted [&_pre]:p-3 [&_pre_code]:p-0 [&_hr]:my-4",
  "[&_table]:my-3 [&_table]:block [&_table]:w-full [&_table]:overflow-x-auto [&_table]:border-collapse [&_td]:min-w-24 [&_td]:border [&_td]:p-2 [&_th]:min-w-24 [&_th]:border [&_th]:bg-muted [&_th]:p-2",
  "[&_ul[data-type=taskList]]:list-none [&_ul[data-type=taskList]]:pl-0 [&_li[data-type=taskItem]]:flex [&_li[data-type=taskItem]]:items-start [&_li[data-type=taskItem]]:gap-2 [&_li[data-type=taskItem]>div]:min-w-0 [&_li[data-type=taskItem]>div]:flex-1 [&_li[data-type=taskItem]>label]:pt-0.5 [&_.task-list-item]:list-none [&_.task-list-item>input]:mr-2",
].join(" ");

export function markdownFence(content: string) {
  return "`".repeat(
    Array.from(content.matchAll(/`+/g)).reduce(
      (length, match) => Math.max(length, match[0].length + 1),
      3,
    ),
  );
}

export function openMarkdownLink(event: MouseEvent, href?: string) {
  if (!opensWebLinks() || !href) return;
  let url: URL;
  try {
    url = new URL(href);
  } catch {
    return;
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") return;
  event.preventDefault();
  void openExternalUrl(url.toString()).catch(() => toast.error("Could not open web link"));
}
