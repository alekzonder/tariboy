/** Raw byte sequences the Terminal tab's hotkey bar sends over the terminal
 * websocket. These are literal terminal escape sequences, not display labels;
 * `useTerminalSocket.send()` writes them straight to the socket. */
export const HOTKEY_BYTES: Record<"esc" | "enter" | "ctrlc" | "up" | "down", string> = {
  esc: "\x1b",
  enter: "\r",
  ctrlc: "\x03",
  up: "\x1b[A",
  down: "\x1b[B",
} as const
