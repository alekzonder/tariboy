import { expect, it } from "vitest"
import generator from "../../.design-sync/make-entry.mjs?raw"
import entry from "../../.design-sync/entry.tsx?raw"
import workspace from "../../.design-sync/previews/TasksWorkspace.tsx?raw"
import detail from "../../.design-sync/previews/TaskDetail.tsx?raw"

it("exports the bundled query context and gives each standalone Tasks preview its own client", () => {
  for (const source of [generator, entry]) {
    expect(source).toContain('export { QueryClientProvider } from "@tanstack/react-query"')
    expect(source).toContain('export { createQueryClient } from "@/lib/queryClient"')
  }
  for (const source of [workspace, detail]) {
    expect(source).toContain("useState(createQueryClient)")
    expect(source).toContain("<QueryClientProvider client={client}>")
  }
})
