import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Link, MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  AppSettings,
  DesktopUpdatesProvider,
  UpdateBanner,
} from "./DesktopUpdates";
import type { DesktopUpdateSnapshot } from "@/lib/desktop";

const bridge = vi.hoisted(() => ({
  desktop: true,
  state: vi.fn(),
  download: vi.fn(),
  install: vi.fn(),
  subscribe: vi.fn(),
  listener: null as ((state: unknown) => void) | null,
  unsubscribe: vi.fn(),
}));

vi.mock("@/lib/desktop", () => ({
  isDesktop: () => bridge.desktop,
  desktopUpdateState: bridge.state,
  desktopUpdateDownload: bridge.download,
  desktopUpdateInstall: bridge.install,
  onDesktopUpdateState: bridge.subscribe,
}));

const AUTO_DOWNLOAD_KEY = "desktop:updates:auto-download:v1";
const SIX_HOURS = 6 * 60 * 60 * 1000;

function update(overrides: Partial<DesktopUpdateSnapshot> = {}): DesktopUpdateSnapshot {
  return {
    revision: 0,
    current_version: "1.2.3",
    phase: "idle",
    version: "",
    downloaded_bytes: 0,
    total_bytes: null,
    error: "",
    ...overrides,
  };
}

function renderUpdates(children = <><AppSettings /><UpdateBanner /></>) {
  return render(
    <MemoryRouter>
      <DesktopUpdatesProvider>{children}</DesktopUpdatesProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  vi.useRealTimers();
  localStorage.clear();
  bridge.desktop = true;
  bridge.listener = null;
  bridge.unsubscribe.mockReset();
  bridge.state.mockReset().mockResolvedValue(update());
  bridge.download.mockReset().mockResolvedValue(update({ revision: 1, phase: "up-to-date" }));
  bridge.install.mockReset().mockResolvedValue(update({ revision: 1, phase: "installing" }));
  bridge.subscribe.mockReset().mockImplementation(
    (listener: (state: unknown) => void, registered: () => void) => {
      bridge.listener = listener;
      void Promise.resolve().then(registered);
      return bridge.unsubscribe;
    },
  );
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("Desktop updates", () => {
  it("allows one manual check while automatic downloads are disabled", async () => {
    localStorage.setItem(AUTO_DOWNLOAD_KEY, "false");
    let finish: (state: DesktopUpdateSnapshot) => void = () => {};
    bridge.download.mockReturnValue(new Promise((resolve) => { finish = resolve; }));
    renderUpdates();

    const button = await screen.findByRole("button", {
      name: "Проверить и скачать обновление",
    });
    fireEvent.click(button);
    fireEvent.click(button);

    expect(bridge.download).toHaveBeenCalledTimes(1);
    expect(button).toBeDisabled();
    await act(async () => finish(update({ revision: 1, phase: "up-to-date" })));
    expect(await screen.findByText("Установлена актуальная версия")).toBeInTheDocument();
  });

  it("checks on startup when automatic downloads use their default", async () => {
    renderUpdates();

    await waitFor(() => expect(bridge.download).toHaveBeenCalledTimes(1));
    expect(bridge.state).toHaveBeenCalledTimes(1);
  });

  it("checks every six hours without catch-up loops after sleep", async () => {
    vi.useFakeTimers();
    renderUpdates();
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(bridge.download).toHaveBeenCalledTimes(1);

    act(() => vi.advanceTimersByTime(SIX_HOURS - 1));
    expect(bridge.download).toHaveBeenCalledTimes(1);
    await act(async () => {
      vi.advanceTimersByTime(1);
      await Promise.resolve();
    });
    expect(bridge.download).toHaveBeenCalledTimes(2);

    await act(async () => {
      vi.advanceTimersByTime(SIX_HOURS * 3);
      await Promise.resolve();
    });
    expect(bridge.download).toHaveBeenCalledTimes(3);
  });

  it("cancels the mounted timer when disabled and leaves it disabled after remount", async () => {
    vi.useFakeTimers();
    const first = renderUpdates();
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(bridge.download).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("switch", {
      name: "Скачивать обновления автоматически",
    }));
    expect(localStorage.getItem(AUTO_DOWNLOAD_KEY)).toBe("false");
    await act(async () => {
      vi.advanceTimersByTime(SIX_HOURS);
      await Promise.resolve();
    });
    expect(bridge.download).toHaveBeenCalledTimes(1);
    first.unmount();

    bridge.download.mockClear();
    renderUpdates();
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(bridge.state).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("switch", {
      name: "Скачивать обновления автоматически",
    })).not.toBeChecked();
    expect(bridge.download).not.toHaveBeenCalled();
  });

  it("keeps the preference usable and reports a storage write failure", async () => {
    localStorage.setItem(AUTO_DOWNLOAD_KEY, "false");
    const storage = localStorage;
    vi.stubGlobal("localStorage", {
      getItem: storage.getItem.bind(storage),
      setItem: () => { throw new Error("storage unavailable"); },
      removeItem: storage.removeItem.bind(storage),
      clear: storage.clear.bind(storage),
      key: storage.key.bind(storage),
      get length() { return storage.length; },
    });
    renderUpdates();

    const toggle = await screen.findByRole("switch", {
      name: "Скачивать обновления автоматически",
    });
    fireEvent.click(toggle);

    expect(toggle).toBeChecked();
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Не удалось сохранить настройку автоматической загрузки",
    );
  });

  it("does not let an older initial snapshot replace a native event", async () => {
    localStorage.setItem(AUTO_DOWNLOAD_KEY, "false");
    let finishState: (state: DesktopUpdateSnapshot) => void = () => {};
    bridge.state.mockReturnValue(new Promise((resolve) => { finishState = resolve; }));
    renderUpdates();
    await waitFor(() => expect(bridge.state).toHaveBeenCalledTimes(1));

    act(() => bridge.listener?.(update({
      revision: 5,
      phase: "downloading",
      version: "2.0.0",
      downloaded_bytes: 2048,
    })));
    await act(async () => finishState(update({ revision: 0 })));

    expect(screen.getByText("Загружено 2048 байт")).toBeInTheDocument();
  });

  it("renders accessible indeterminate progress when the total is unknown", async () => {
    localStorage.setItem(AUTO_DOWNLOAD_KEY, "false");
    bridge.state.mockResolvedValue(update({
      revision: 2,
      phase: "downloading",
      version: "2.0.0",
      downloaded_bytes: 512,
      total_bytes: null,
    }));
    renderUpdates();

    const progress = await screen.findByRole("progressbar", { name: "Загрузка обновления" });
    expect(progress).not.toHaveAttribute("max");
    expect(progress).not.toHaveAttribute("value");
    expect(screen.getByText("Загружено 512 байт")).toBeInTheDocument();
  });

  it("reports signature failure without claiming an update is ready", async () => {
    localStorage.setItem(AUTO_DOWNLOAD_KEY, "false");
    bridge.state.mockResolvedValue(update({
      revision: 3,
      phase: "error",
      version: "2.0.0",
      error: "Проверка подписи обновления завершилась ошибкой.",
    }));
    renderUpdates();

    expect(await screen.findByRole("alert")).toHaveTextContent("Проверка подписи");
    expect(screen.queryByText("Версия 2.0.0 загружена")).toBeNull();
  });

  it("offers installation only for a ready verified package", async () => {
    localStorage.setItem(AUTO_DOWNLOAD_KEY, "false");
    bridge.state.mockResolvedValue(update({ revision: 4, phase: "ready", version: "2.0.0" }));
    bridge.install.mockReturnValue(new Promise(() => {}));
    renderUpdates();

    expect(await screen.findByText("Версия 2.0.0 загружена")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Обновить" }));

    expect(bridge.install).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("status")).toHaveTextContent("Установка обновления");
  });

  it("shows an install failure and allows retry with the retained package", async () => {
    localStorage.setItem(AUTO_DOWNLOAD_KEY, "false");
    const failed = update({
      revision: 5,
      phase: "ready",
      version: "2.0.0",
      error: "Не удалось установить обновление. Повторите попытку.",
    });
    bridge.state.mockResolvedValue(update({ revision: 4, phase: "ready", version: "2.0.0" }));
    bridge.install.mockResolvedValue(failed);
    renderUpdates();

    fireEvent.click(await screen.findByRole("button", { name: "Обновить" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Не удалось установить");
    fireEvent.click(screen.getByRole("button", { name: "Обновить" }));

    await waitFor(() => expect(bridge.install).toHaveBeenCalledTimes(2));
  });

  it("keeps one provider and snapshot across route changes", async () => {
    localStorage.setItem(AUTO_DOWNLOAD_KEY, "false");
    bridge.state.mockResolvedValue(update({ revision: 4, phase: "ready", version: "2.0.0" }));
    renderUpdates(
      <>
        <nav><Link to="/">Рабочая область</Link><Link to="/app-settings">Настройки</Link></nav>
        <Routes>
          <Route path="/" element={<UpdateBanner />} />
          <Route path="/app-settings" element={<AppSettings />} />
        </Routes>
      </>,
    );

    expect(await screen.findByText("Версия 2.0.0 загружена")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("link", { name: "Настройки" }));

    expect(await screen.findByRole("heading", { name: "Настройки приложения" }))
      .toBeInTheDocument();
    expect(screen.getByText("1.2.3")).toBeInTheDocument();
    expect(bridge.subscribe).toHaveBeenCalledTimes(1);
    expect(bridge.state).toHaveBeenCalledTimes(1);
  });

  it("makes no native or network update call in browser mode", () => {
    bridge.desktop = false;
    renderUpdates();

    expect(screen.getByText("Обновления доступны только в приложении Desktop."))
      .toBeInTheDocument();
    expect(bridge.subscribe).not.toHaveBeenCalled();
    expect(bridge.state).not.toHaveBeenCalled();
    expect(bridge.download).not.toHaveBeenCalled();
    expect(bridge.install).not.toHaveBeenCalled();
  });
});
