import type { Operation, OperationList, OperationLog } from "../api-types.generated";
import type { Transport } from "./client";

export type {
  Operation,
  OperationList,
  OperationLog,
  OperationStarted,
} from "../api-types.generated";

/** What the UI sends as `initiator` when it starts a background operation. */
export const UI_INITIATOR = "ui";

const operationPath = (id: number) => `/api/operations/${encodeURIComponent(String(id))}`;

export function operationsApi(t: Transport) {
  return {
    /** One page of operations, newest first. `limit` is at most 200. */
    list: (page: number, limit: number, signal?: AbortSignal) =>
      t.get<OperationList>(`/api/operations?page=${page}&limit=${limit}`, signal),
    get: (id: number, signal?: AbortSignal) => t.get<Operation>(operationPath(id), signal),
    /** Up to 1 MiB of the log from `offset`; read again from `next` while `next < size`. */
    log: (id: number, offset: number, signal?: AbortSignal) =>
      t.get<OperationLog>(`${operationPath(id)}/log?offset=${offset}`, signal),
    /** The whole log as a text file. Not in /api/openapi.json: a raw download. */
    downloadLog: async (id: number) =>
      (await t.raw("GET", `${operationPath(id)}/log/download`)).blob(),
  };
}
