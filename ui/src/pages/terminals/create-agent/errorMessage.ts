import { ApiError } from "@/lib/api";

export function errorMessage(error: unknown): string {
  return error instanceof ApiError ? error.message : String(error);
}
