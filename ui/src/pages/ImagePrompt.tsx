import { useEffect, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeHighlight from "rehype-highlight";
import { ApiError, imagePromptGet } from "@/lib/api";
import { useImageContext } from "@/components/ImageLayout";
import "@/styles/hljs-github.css";

// Read-only render of an image's assembled prompt as markdown, using the same
// renderer stack (remark-gfm + rehype-highlight, hljs-github theme) the
// FileBrowser preview uses.
export default function ImagePrompt() {
  const { ref, hostKey } = useImageContext();
  const [prompt, setPrompt] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    setError("");
    setPrompt("");
    void imagePromptGet(ref)
      .then((r) => setPrompt(r.prompt))
      .catch((e) => setError(e instanceof ApiError ? e.message : String(e)));
  }, [hostKey, ref]);

  if (error) return <p className="text-sm text-destructive">{error}</p>;
  return (
    <div data-testid="md-preview" className="prose prose-sm dark:prose-invert max-w-none">
      <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeHighlight]}>
        {prompt}
      </ReactMarkdown>
    </div>
  );
}
