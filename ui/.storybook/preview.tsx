import type { Preview } from "@storybook/react-vite";
import {
  withAppProviders,
  withDaemon,
  withQueryClient,
  withRoute,
  withTheme,
} from "../src/storybook/decorators";
import "../src/index.css";

// Storybook applies the last decorator outermost: withTheme wraps everything,
// and withRoute sits inside withAppProviders, which then skips its own router.
const preview: Preview = {
  decorators: [withQueryClient, withDaemon, withRoute, withAppProviders, withTheme],
  // The sidebar reads from the tokens up to the pages. The other titles come
  // from the file paths, so a story id does not change when this order does.
  parameters: {
    options: {
      storySort: {
        order: [
          "Foundations",
          ["Colors", "Typography", "Shape", "Recipes"],
          "Components",
          ["Ui", "*"],
          "Pages",
          "Mockups",
        ],
      },
    },
  },
  globalTypes: {
    theme: {
      description: "Color theme",
      toolbar: {
        title: "Theme",
        icon: "circlehollow",
        items: [
          { value: "light", title: "Light" },
          { value: "dark", title: "Dark" },
        ],
        dynamicTitle: true,
      },
    },
  },
  initialGlobals: { theme: "light" },
};

export default preview;
