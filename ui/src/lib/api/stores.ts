import type { StoreDetail, StoreRemoveResult, StoreView } from "../api-types.generated";
import type { Transport } from "./client";
import { UI_INITIATOR, type OperationStarted } from "./operations";

export type {
  StoreDetail,
  StoreImageView,
  StoreView,
  StoreWorkflowView,
} from "../api-types.generated";

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
    /** Starts a background build; the result arrives on the returned operation. */
    buildStoreImage: (input: { source: string; name?: string; tag?: string }) =>
      t.post<OperationStarted>("/api/images/build", { ...input, initiator: UI_INITIATOR }),
    buildStoreWorkflow: (source: string) =>
      t.post<OperationStarted>("/api/workflow-images/build", { source, initiator: UI_INITIATOR }),
  };
}
