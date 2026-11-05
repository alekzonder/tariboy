import type { Meta, StoryObj } from "@storybook/react-vite";
import { HostStatus } from "./HostStatus";

// HostStatus renders only for kind: "ssh" hosts (it returns null otherwise),
// so every fixture below is an ssh host. Shapes follow DaemonMeta in
// src/lib/daemons.ts.
const base = {
  id: "build-01",
  label: "build-01",
  baseURL: "https://10.0.4.11:7777",
  kind: "ssh" as const,
};

const meta = { component: HostStatus } satisfies Meta<typeof HostStatus>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Ready: Story = {
  args: {
    host: {
      ...base,
      state: "ready",
      platform: "Linux",
      arch: "x86_64",
      lastDaemonVersion: "0.58.1",
    },
    appVersion: "0.58.1",
  },
};

export const UpdateAvailable: Story = {
  args: {
    host: {
      ...base,
      state: "ready",
      platform: "Linux",
      arch: "x86_64",
      lastDaemonVersion: "0.57.4",
    },
    appVersion: "0.58.1",
    onUpdate: () => {},
  },
};

export const Disconnected: Story = {
  args: {
    host: { ...base, state: "disconnected", platform: "Linux", arch: "x86_64" },
    appVersion: "0.58.1",
    onConnect: () => {},
  },
};

export const NeedsAuth: Story = {
  args: {
    host: { ...base, state: "needs_auth", platform: "Linux", arch: "x86_64" },
    appVersion: "0.58.1",
    onUpdate: () => {},
  },
};

export const InstallBlocked: Story = {
  args: {
    host: {
      ...base,
      id: "mac-mini",
      label: "mac-mini",
      state: "failed",
      platform: "Darwin",
      arch: "arm64",
      prerequisites: ["Linux", "x86_64 or aarch64", "python3"],
    },
    appVersion: "0.58.1",
    onConnect: () => {},
  },
};
