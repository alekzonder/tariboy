import { render, screen } from "@testing-library/react"
import { expect, it } from "vitest"
import { Dialog, DialogContent, DialogTitle } from "./dialog"

it("restyles one dialog's scrim without letting it stop covering the viewport", () => {
  render(
    <Dialog open>
      <DialogContent overlayClassName="bg-[color-mix(in_oklab,var(--foreground)_5%,transparent)]">
        <DialogTitle>Sheet</DialogTitle>
      </DialogContent>
    </Dialog>,
  )
  const overlay = document.querySelector('[data-slot="dialog-overlay"]')!
  // The caller's tint wins over the default one...
  expect(overlay.className).toContain("bg-[color-mix(in_oklab,var(--foreground)_5%,transparent)]")
  expect(overlay.className).not.toContain("bg-black/10")
  // ...but the scrim still spans the whole window, topbar included.
  expect(overlay.className).toContain("fixed")
  expect(overlay.className).toContain("inset-0")
  expect(screen.getByText("Sheet")).toBeInTheDocument()
})
