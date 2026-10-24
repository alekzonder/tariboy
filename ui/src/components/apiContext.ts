import { createContext, useContext } from "react";
import type { ApiClient } from "@/lib/api";

export const ApiContext = createContext<ApiClient | null>(null);

// useApi returns the client of the nearest ApiProvider: the server every
// request below that boundary addresses.
export function useApi(): ApiClient {
  const client = useContext(ApiContext);
  if (!client) throw new Error("useApi must be used within an ApiProvider");
  return client;
}
