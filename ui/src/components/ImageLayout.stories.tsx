import type { Meta, StoryObj } from "@storybook/react-vite";
import { ImageLayout } from "./ImageLayout";

// ImageLayout is the shell for one image detail route: a header carrying the
// image ref, its terminal-only badge, digest and build time, the Run Agent /
// provenance / Remove actions, the four-tab strip, and an <Outlet/> for the
// active tab. It resolves `name:tag` from useParams(), so it is mounted inside
// a MATCHED route (parameters.tariboy.route). With no daemon the fetches
// reject and the body shows the real error branch, with the header, actions
// and tabs still in place.

const meta = {
  component: ImageLayout,
  parameters: {
    tariboy: {
      daemon: true,
      route: { path: "/library/images/worker/v1", pattern: "/library/images/:name/:tag/*" },
    },
  },
  decorators: [
    (Story) => (
      <div className="h-82.5 w-210 overflow-hidden rounded-card border border-border bg-background">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ImageLayout>;
export default meta;

type Story = StoryObj<typeof meta>;

export const ImageShell: Story = { args: { hostId: "build-01" } };
