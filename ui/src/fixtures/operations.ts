import type { Operation, OperationStarted } from "@/types";

import { minutesAgo } from "./time";

export const operation = (over: Partial<Operation> = {}): Operation => ({
  id: 1,
  kind: "image.build",
  title: "Build image reviewer:latest",
  status: "running",
  initiator: "ui",
  params: {},
  created_at: minutesAgo(5),
  ...over,
});

/** What a build POST answers: the operation that now runs the build. */
export const operationStarted = (over: Partial<Operation> = {}): OperationStarted => ({
  operation: operation(over),
});

/** Edge sets of operations: `empty`, `one`, `many` (50) and `longText`. */
export const operations = {
  empty: [] as Operation[],
  one: [operation()],
  many: Array.from({ length: 50 }, (_, i) =>
    operation({
      id: i + 1,
      title: `Build image worker-${i + 1}:latest`,
      created_at: minutesAgo(i + 1),
    }),
  ),
  longText: [
    operation({
      title:
        "Build image a-very-long-image-name-that-has-to-truncate-in-the-operations-list:latest",
    }),
  ],
};
