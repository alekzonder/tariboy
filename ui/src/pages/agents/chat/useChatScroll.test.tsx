import { afterEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { useChatScroll } from "./useChatScroll";

afterEach(() => vi.restoreAllMocks());

it("leaves the reading position alone when messages arrive away from the bottom", () => {
  const scroll = vi.spyOn(HTMLElement.prototype, "scrollIntoView");
  function Feed({ count }: { count: number }) {
    const { bottom, atBottom, onScroll } = useChatScroll(count);
    return (
      <div data-testid="feed" onScroll={onScroll}>
        <span>{atBottom ? "Following" : "Reading older messages"}</span>
        <div ref={bottom} />
      </div>
    );
  }
  const view = render(<Feed count={1} />);
  expect(scroll).toHaveBeenCalledTimes(1);
  const feed = screen.getByTestId("feed");
  Object.defineProperties(feed, {
    scrollHeight: { value: 1000 },
    clientHeight: { value: 200 },
    scrollTop: { value: 100, writable: true },
  });
  fireEvent.scroll(feed);
  view.rerender(<Feed count={2} />);
  expect(screen.getByText("Reading older messages")).toBeInTheDocument();
  expect(scroll).toHaveBeenCalledTimes(1);
  feed.scrollTop = 800;
  fireEvent.scroll(feed);
  expect(scroll).toHaveBeenCalledTimes(1);
  view.rerender(<Feed count={3} />);
  expect(scroll).toHaveBeenCalledTimes(2);
});
