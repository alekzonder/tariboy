/** The URL a Markdown link may point to: a complete http:// or https:// URL, else null. */
export function parseLinkURL(text: string): URL | null {
  try {
    const url = new URL(text);
    return ["http:", "https:"].includes(url.protocol) ? url : null;
  } catch {
    return null;
  }
}
