import { defineConfig } from "blume";

export default defineConfig({
  title: "Tariboy",
  description:
    "The control plane for autonomous coding agents — desktop workflows, operations, architecture, and reference.",
  logo: { image: { light: "/favicon.svg", alt: "Tariboy" } },
  deployment: {
    base: "/tariboy",
    site: "https://alekzonder.github.io",
  },
  github: {
    owner: "alekzonder",
    repo: "tariboy",
    dir: "docs",
  },
  navigation: {
    sidebar: {
      display: "group",
    },
  },
});
