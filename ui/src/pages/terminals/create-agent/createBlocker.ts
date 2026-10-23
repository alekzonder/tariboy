interface CreateBlocker {
  message: string;
  error: boolean;
  retryImage?: boolean;
}

// createBlocker names the first unmet precondition of creation, so the
// disabled Create agent button always carries its reason.
export function createBlocker(state: {
  sourceState: "idle" | "loading" | "ready" | "error";
  hostLabel: string;
  targetIsCurrent: boolean;
  imagesError: string;
  image: string;
  imageAvailable: boolean;
  manifestError: string;
  manifestIsCurrent: boolean;
}): CreateBlocker | null {
  if (state.sourceState === "loading")
    return { message: "Loading source configuration…", error: false };
  if (state.sourceState === "error")
    return { message: "Source configuration did not load", error: false };
  if (!state.targetIsCurrent)
    return {
      message: `Loading built images on ${state.hostLabel}…`,
      error: false,
    };
  if (state.imagesError) return { message: state.imagesError, error: true };
  if (!state.image) return { message: "Choose an image", error: false };
  if (!state.imageAvailable)
    return {
      message: `Image ${state.image} is not built on ${state.hostLabel}`,
      error: true,
    };
  if (state.manifestError)
    return { message: state.manifestError, error: true, retryImage: true };
  if (!state.manifestIsCurrent)
    return {
      message: `Loading image ${state.image} on ${state.hostLabel}…`,
      error: false,
    };
  return null;
}
