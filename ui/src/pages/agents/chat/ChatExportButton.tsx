import { Download } from "lucide-react";
import type { ChatMessage } from "@/lib/api";

function transcript(messages: ChatMessage[], agent: string): string {
  return [
    `# Chat with ${agent}`,
    "",
    ...messages.map((message) => `## ${message.from} · ${message.ts}\n\n${message.text}\n`),
  ].join("\n");
}

/** Downloads the shown conversation as a Markdown transcript. */
export function ChatExportButton({ messages, name }: { messages: ChatMessage[]; name: string }) {
  function exportTranscript() {
    const url = URL.createObjectURL(
      new Blob([transcript(messages, name)], { type: "text/markdown" }),
    );
    const link = document.createElement("a");
    link.href = url;
    link.download = `chat-${name}.md`;
    link.click();
    URL.revokeObjectURL(url);
  }

  return (
    <button
      type="button"
      aria-label="Export transcript"
      title="Export transcript"
      onClick={exportTranscript}
      disabled={messages.length === 0}
      className="grid size-6.5 place-items-center rounded-control text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-50"
    >
      <Download className="size-3.5" />
    </button>
  );
}
