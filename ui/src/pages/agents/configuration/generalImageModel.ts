// The pure half of the General block's image section: links, labels and the
// update notice. GeneralImage renders them.
import type { AgentImageStatus } from "@/lib/api";
import { hostToParam } from "@/lib/terminalsHost";

type NextReason = NonNullable<AgentImageStatus["next"]["reason"]>;

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

const NEXT_REASONS: Record<NextReason, string> = {
  pending: "Explicit pending assignment",
  ref_moved: "Content the active ref points at now",
  current: "Current pinned image",
};

// nextReasonText says why the next iteration resolves to its image.
export const nextReasonText = (reason: AgentImageStatus["next"]["reason"]) =>
  (reason && NEXT_REASONS[reason]) || NEXT_REASONS.current;

// updatesOnNext is true when the next iteration resolves to known content
// that differs from the current image.
export function updatesOnNext(imageStatus: AgentImageStatus): boolean {
  const next = imageStatus.next;
  return Boolean(next && !next.error && next.digest && next.digest !== imageStatus.current.digest);
}

// imageSides lists the current and next images under their labels; a stopped
// agent's current image is the version it activated.
export function imageSides(imageStatus: AgentImageStatus, stopped: boolean) {
  return [
    { label: stopped ? "Activated version" : "Current", value: imageStatus.current },
    { label: "Next iteration", value: imageStatus.next },
  ] as const;
}
