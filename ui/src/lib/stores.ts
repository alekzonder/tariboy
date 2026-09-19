import { apiOn, type ImageBuildResult } from "@/lib/api";
import type { Daemon } from "@/lib/daemons";

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

export interface StoreDetail extends Store {
  images: StoreImage[];
  auto?: StoreAuto;
}

type Target = Daemon | null;
const storePath = (name: string) => `/api/stores/${encodeURIComponent(name)}`;

export const listStores = (target: Target) => apiOn<Store[]>(target, "GET", "/api/stores");
export const addStore = (target: Target, input: { name: string; source: string }) =>
  apiOn<Store>(target, "POST", "/api/stores", input);
export const getStore = (target: Target, name: string) =>
  apiOn<StoreDetail>(target, "GET", storePath(name));
export const refreshStore = (target: Target, name: string) =>
  apiOn<StoreDetail>(target, "POST", `${storePath(name)}/refresh`);
export const setStoreAuto = (target: Target, name: string, input: { interval: number; image: string[] }) =>
  apiOn<StoreDetail>(target, "POST", `${storePath(name)}/auto`, input);
export const removeStore = (target: Target, name: string) =>
  apiOn<{ removed: boolean }>(target, "DELETE", storePath(name));
export const buildStoreImage = (target: Target, input: { source: string; name?: string; tag?: string }) =>
  apiOn<ImageBuildResult>(target, "POST", "/api/images/build", input);
