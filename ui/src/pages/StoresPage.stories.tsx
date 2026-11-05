import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react-vite";
import { withWidth } from "@/storybook/decorators";
import StoresPage from "./StoresPage";

// One component, two screens: the Stores list (no `name`) and one Store. The
// mounted paths are App.tsx's /library/sources and /library/sources/:name.
// There is no daemon behind Storybook, so both show their honest failure line.
const BASE = "/library/sources";

const withFrame: Decorator = (Story) => (
  <div className="h-[620px] overflow-hidden rounded-md border border-border bg-background">
    <Story />
  </div>
);

const meta = {
  component: StoresPage,
  decorators: [withFrame, withWidth(1024)],
  parameters: { layout: "fullscreen", tariboy: { daemon: true } },
} satisfies Meta<typeof StoresPage>;
export default meta;

type Story = StoryObj;

export const StoreList: Story = {
  parameters: { tariboy: { daemon: true, route: { path: BASE, pattern: BASE } } },
  render: () => <StoresPage basePath={BASE} />,
};

export const StoreDetail: Story = {
  parameters: {
    tariboy: {
      daemon: true,
      route: { path: `${BASE}/tariboy-images`, pattern: `${BASE}/:name` },
    },
  },
  render: () => <StoresPage name="tariboy-images" basePath={BASE} />,
};
