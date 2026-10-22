import type { ReactElement, PropsWithChildren } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import { render as testingRender, type RenderOptions } from "@testing-library/react"
import { afterEach } from "vitest"
import { createQueryClient } from "@/lib/queryClient"

export { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"

const clients: ReturnType<typeof createQueryClient>[] = []
afterEach(() => { for (const client of clients.splice(0)) client.clear() })

export function render(element: ReactElement, options: RenderOptions = {}) {
  const client = createQueryClient()
  clients.push(client)
  const Outer = options.wrapper
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}>{Outer ? <Outer>{children}</Outer> : children}</QueryClientProvider>
  )
  return { ...testingRender(element, { ...options, wrapper }), client }
}
