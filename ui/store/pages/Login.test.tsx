import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import Login from "./Login";

vi.mock("../lib/storeApi", () => ({
  setToken: vi.fn(),
  clearToken: vi.fn(),
  probeAuth: vi.fn(),
}));
import { setToken, clearToken, probeAuth } from "../lib/storeApi";

afterEach(() => vi.clearAllMocks());

describe("Login", () => {
  it("stores the token and calls onAuthed when the probe succeeds", async () => {
    (probeAuth as ReturnType<typeof vi.fn>).mockResolvedValue(true);
    const onAuthed = vi.fn();
    render(<Login onAuthed={onAuthed} />);
    await userEvent.type(screen.getByLabelText(/bearer token/i), "s3cr3t");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));
    await waitFor(() => expect(onAuthed).toHaveBeenCalled());
    expect(setToken).toHaveBeenCalledWith("s3cr3t");
  });

  it("clears the token and shows an error when the probe fails", async () => {
    (probeAuth as ReturnType<typeof vi.fn>).mockResolvedValue(false);
    const onAuthed = vi.fn();
    render(<Login onAuthed={onAuthed} />);
    await userEvent.type(screen.getByLabelText(/bearer token/i), "wrong");
    await userEvent.click(screen.getByRole("button", { name: /sign in/i }));
    await waitFor(() => expect(clearToken).toHaveBeenCalled());
    expect(onAuthed).not.toHaveBeenCalled();
    expect(screen.getByText(/invalid token/i)).toBeInTheDocument();
  });

  it("renders the token input masked (never shown as plain text)", async () => {
    (probeAuth as ReturnType<typeof vi.fn>).mockResolvedValue(true);
    render(<Login onAuthed={vi.fn()} />);
    const input = screen.getByLabelText(/bearer token/i) as HTMLInputElement;
    expect(input.type).toBe("password");
  });
});
