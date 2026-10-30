// The pure half of the image subsection and the About line: the short digest
// and the Library link. GeneralImage and the page render them.
import { hostToParam } from "@/lib/terminalsHost";

// A digest is a 64-hex-character hash: shown whole it dominates the identity
// row and wraps across lines. Keep enough of both ends to compare against a
// registry listing by eye; the untruncated value stays in the a11y tree.
export function shortDigest(digest: string): string {
  return digest.length > 24 ? `${digest.slice(0, 14)}…${digest.slice(-8)}` : digest;
}

// imageHref links an image ref ("name:tag") to its Library page; a ref
// without a tag links to the image list. The link names the target host only
// when the workspace has several.
export function imageHref(targetId: string, ref: string, multiHost: boolean): string {
  const split = ref.lastIndexOf(":");
  const path =
    split < 0
      ? "/library/images"
      : `/library/images/${encodeURIComponent(ref.slice(0, split))}/${encodeURIComponent(ref.slice(split + 1))}`;
  return multiHost ? `${path}?host=${encodeURIComponent(hostToParam(targetId))}` : path;
}
