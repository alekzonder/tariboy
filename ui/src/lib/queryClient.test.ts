import { afterEach, expect, it, vi } from "vitest";
import { queryClient } from "./queryClient";

afterEach(() => vi.restoreAllMocks());

it("rereads active queries when the app comes back", () => {
  const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
  document.dispatchEvent(new Event("visibilitychange"));
  expect(invalidate).toHaveBeenCalledOnce();
  expect(invalidate).toHaveBeenCalledWith();
});
