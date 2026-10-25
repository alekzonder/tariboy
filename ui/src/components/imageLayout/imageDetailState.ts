type ImageDetailBodyState = "error" | "loaded" | "loading";

const BODY_STATES: {
  when: (error: string, hasDetail: boolean) => boolean;
  state: ImageDetailBodyState;
}[] = [
  { when: (error) => !!error, state: "error" },
  { when: (_error, hasDetail) => hasDetail, state: "loaded" },
  { when: () => true, state: "loading" },
];

export function imageDetailBodyState(error: string, hasDetail: boolean): ImageDetailBodyState {
  return BODY_STATES.find((row) => row.when(error, hasDetail))!.state;
}
