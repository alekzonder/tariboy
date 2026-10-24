import type { ReactElement, PropsWithChildren } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import { render as testingRender, type RenderOptions } from "@testing-library/react"
import { afterEach } from "vitest"
import { createQueryClient } from "@/lib/queryClient"
import { ApiProvider } from "@/components/ApiProvider"
import { createApi, type ApiClient } from "@/lib/api"

export { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"

const clients: ReturnType<typeof createQueryClient>[] = []
afterEach(() => { for (const client of clients.splice(0)) client.clear() })

export function render(element: ReactElement, options: RenderOptions & { api?: ApiClient } = {}) {
  const { api = createApi(null), ...renderOptions } = options
  const client = createQueryClient()
  clients.push(client)
  const Outer = renderOptions.wrapper
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={client}><ApiProvider client={api}>{Outer ? <Outer>{children}</Outer> : children}</ApiProvider></QueryClientProvider>
  )
  return { ...testingRender(element, { ...renderOptions, wrapper }), client }
}
