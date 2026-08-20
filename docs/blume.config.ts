import { defineConfig } from "blume";

export default defineConfig({
  title: "Tariboy",
  description:
    "The control plane for autonomous coding agents — desktop workflows, operations, architecture, and reference.",
  deployment: {
    base: "/tariboy",
    site: "https://alekzonder.github.io",
  },
  navigation: {
    sidebar: {
      display: "group",
    },
  },
});
