import type { StorybookConfig } from "@storybook/react-vite";

// No addons and no viteFinal: the Vite file below has no /api proxy, and
// nothing here may add one (a test pins this).
const config: StorybookConfig = {
  stories: ["../src/**/*.stories.tsx"],
  framework: {
    name: "@storybook/react-vite",
    options: { builder: { viteConfigPath: "vite.storybook.config.ts" } },
  },
};

export default config;
