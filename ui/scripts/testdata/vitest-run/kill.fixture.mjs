import { it } from "vitest";

it("kills the vitest process in the middle of the run", () => {
  process.kill(process.pid, "SIGKILL");
});
