import type { StoreDetail, StoreRemoveResult, StoreView } from "../api-types.generated";
import type { Transport } from "./client";
import type { ImageBuildResult } from "./images";
import type { WorkflowBuildResult } from "./workflows";

export type {
  StoreDetail,
  StoreImageView,
  StoreView,
  StoreWorkflowView,
} from "../api-types.generated";

// The public Tariboy Store, offered on the Stores page as a one-click registration.
export const TARIBOY_STORE = {
  name: "tariboy",
  source: "https://github.com/alekzonder/tariboy-store.git",
};

const storePath = (name: string) => `/api/stores/${encodeURIComponent(name)}`;

export function storesApi(t: Transport) {
  return {
    listStores: () => t.get<StoreView[]>("/api/stores"),
    addStore: (input: { name: string; source: string }) => t.post<StoreView>("/api/stores", input),
    getStore: (name: string) => t.get<StoreDetail>(storePath(name)),
    refreshStore: (name: string) => t.post<StoreDetail>(`${storePath(name)}/refresh`),
    setStoreAuto: (name: string, input: { interval: number; image: string[] }) =>
      t.post<StoreDetail>(`${storePath(name)}/auto`, input),
    removeStore: (name: string) => t.delete<StoreRemoveResult>(storePath(name)),
    buildStoreImage: (input: { source: string; name?: string; tag?: string }) =>
      t.post<ImageBuildResult>("/api/images/build", input),
    buildStoreWorkflow: (source: string) =>
      t.post<WorkflowBuildResult>("/api/workflow-images/build", { source }),
  };
}
