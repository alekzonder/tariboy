import { fireEvent, render, screen } from "@testing-library/react"
import { expect, it, vi } from "vitest"
import { Segmented } from "./segmented"

const options = [
  { value: "a", label: "Alpha" },
  { value: "b", label: "Beta" },
  { value: "c", label: "Gamma" },
] as const

it("moves the selection with the arrow keys and wraps at the ends", () => {
  const onChange = vi.fn()
  render(<Segmented label="Pick" options={options} value="a" onChange={onChange} />)

  fireEvent.keyDown(screen.getByRole("radio", { name: "Alpha" }), { key: "ArrowRight" })
  expect(onChange).toHaveBeenLastCalledWith("b")
  fireEvent.keyDown(screen.getByRole("radio", { name: "Alpha" }), { key: "ArrowLeft" })
  expect(onChange).toHaveBeenLastCalledWith("c")
})
