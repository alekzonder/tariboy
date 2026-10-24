import type { Transport } from "./client";
import type { ImageBuildResult } from "./images";
import type { WorkflowBuildResult } from "./workflows";

export interface Store {
  name: string;
  source: string;
  path: string;
}

export interface StoreImage {
  name: string;
  version: string;
  built_version: string;
  update_needed: boolean;
  error?: string;
  latest_status: "built" | "missing" | "unversioned" | "error";
  latest_error?: string;
}

export interface StoreAuto {
  interval_minutes: number;
  images: string[];
}

export interface StoreWorkflow {
  name: string;
  version: string;
  error?: string;
}

export interface StoreDetail extends Store {
  images: StoreImage[];
  workflows?: StoreWorkflow[];
  auto?: StoreAuto;
}

// The public Tariboy Store, offered on the Stores page as a one-click registration.
export const TARIBOY_STORE = { name: "tariboy", source: "https://github.com/alekzonder/tariboy-store.git" };

const storePath = (name: string) => `/api/stores/${encodeURIComponent(name)}`;

export function storesApi(t: Transport) {
  return {
    listStores: () => t.get<Store[]>("/api/stores"),
    addStore: (input: { name: string; source: string }) => t.post<Store>("/api/stores", input),
    getStore: (name: string) => t.get<StoreDetail>(storePath(name)),
    refreshStore: (name: string) => t.post<StoreDetail>(`${storePath(name)}/refresh`),
    setStoreAuto: (name: string, input: { interval: number; image: string[] }) =>
      t.post<StoreDetail>(`${storePath(name)}/auto`, input),
    removeStore: (name: string) => t.delete<{ removed: boolean }>(storePath(name)),
    buildStoreImage: (input: { source: string; name?: string; tag?: string }) =>
      t.post<ImageBuildResult>("/api/images/build", input),
    buildStoreWorkflow: (source: string) =>
      t.post<WorkflowBuildResult>("/api/workflow-images/build", { source }),
  };
}
export type StoresApi = ReturnType<typeof storesApi>;
