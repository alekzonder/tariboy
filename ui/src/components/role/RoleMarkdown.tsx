import { MarkdownContent } from "@/pages/tasks/TaskMarkdown";

/** A ROLE.md as Markdown. Its own module, so a form loads react-markdown only
 *  when the reader opens the full text. */
export default function RoleMarkdown({ text }: { text: string }) {
  return <MarkdownContent variant="role">{text}</MarkdownContent>;
}
