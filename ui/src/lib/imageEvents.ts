// Tells image lists that a build in this window finished, so they reload.
// Builders call notifyImagesChanged; lists subscribe. The link is an import,
// not a window event name. Subscribers reload their own current host.
// shortcut: in-window only; switch to query invalidation once the image lists
// load through TanStack Query.

const listeners = new Set<() => void>();

export function notifyImagesChanged(): void {
  for (const listener of [...listeners]) {
    try {
      listener();
    } catch (error) {
      // One broken list must not keep the others stale.
      console.error(error);
    }
  }
}

export function subscribeImagesChanged(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
