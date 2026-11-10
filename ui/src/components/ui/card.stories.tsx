import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge } from "./badge";
import { Button } from "./button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "./card";

const meta = { component: Card } satisfies Meta<typeof Card>;
export default meta;

type Story = StoryObj;

export const ScriptEditorCard: Story = {
  render: () => (
    <Card className="max-w-140">
      <CardHeader>
        <CardTitle>Global Agent Shell Script</CardTitle>
        <CardDescription>
          Bash commands sourced before every agent iteration on this host.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <pre className="m-0 whitespace-pre-wrap rounded-card border border-border px-2.5 py-2 font-mono text-body leading-5">
          {`export TARIBOY_PROFILE=team\nsource /srv/tariboy/env/shared.sh`}
        </pre>
      </CardContent>
      <CardFooter className="justify-end gap-2">
        <Button variant="outline">Discard changes</Button>
        <Button>Save script</Button>
      </CardFooter>
    </Card>
  ),
};

export const WithAction: Story = {
  render: () => (
    <Card size="sm" className="max-w-105">
      <CardHeader>
        <CardTitle>worker:v2</CardTitle>
        <CardDescription>Built 2026-09-11 09:47 · sha256:9ac71e3f52</CardDescription>
        <CardAction>
          <Badge variant="secondary">Pending</Badge>
        </CardAction>
      </CardHeader>
      <CardContent className="text-muted-foreground">
        Scheduled for builder on the next iteration. Runtime settings are unchanged.
      </CardContent>
    </Card>
  ),
};

export const ContentOnly: Story = {
  render: () => (
    <Card className="max-w-105">
      <CardContent>
        <div className="font-medium">No built images on this host.</div>
        <div className="text-muted-foreground">
          Build one from a directory containing Tariboyfile.yaml.
        </div>
      </CardContent>
    </Card>
  ),
};
