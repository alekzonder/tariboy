import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react-vite";
import { withWidth } from "@/storybook/decorators";
import LibraryImagesPage from "./LibraryImagesPage";

// The Library's Images tab: toolbar over one row per image name. Every name is
// a <Link>, so it mounts at App.tsx's /library/images. There is no daemon
// behind Storybook, so the page shows its error line in place of the table.
const withFrame: Decorator = (Story) => (
  <div className="h-[620px] overflow-hidden rounded-md border border-border bg-background">
    <Story />
  </div>
);

const meta = {
  component: LibraryImagesPage,
  decorators: [withFrame, withWidth(1024)],
  parameters: {
    layout: "fullscreen",
    tariboy: {
      daemon: true,
      route: { path: "/library/images", pattern: "/library/images" },
    },
  },
} satisfies Meta<typeof LibraryImagesPage>;
export default meta;

type Story = StoryObj;

export const Images: Story = {
  render: () => <LibraryImagesPage hostId="" />,
};
