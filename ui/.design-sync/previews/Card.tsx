import {
  Badge,
  Button,
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "tariboy-ui";

export const ScriptEditorCard = () => (
  <Card style={{ maxWidth: 560 }}>
    <CardHeader>
      <CardTitle>Global Agent Shell Script</CardTitle>
      <CardDescription>
        Bash commands sourced before every agent iteration on this host.
      </CardDescription>
    </CardHeader>
    <CardContent>
      <pre
        style={{
          margin: 0,
          padding: "8px 10px",
          border: "1px solid var(--border)",
          borderRadius: 10,
          font: "13px/20px ui-monospace, SFMono-Regular, Menlo, monospace",
          whiteSpace: "pre-wrap",
        }}
      >
        {`export TARIBOY_PROFILE=team\nsource /srv/tariboy/env/shared.sh`}
      </pre>
    </CardContent>
    <CardFooter style={{ justifyContent: "flex-end", gap: 8 }}>
      <Button variant="outline">Discard changes</Button>
      <Button>Save script</Button>
    </CardFooter>
  </Card>
);

export const WithAction = () => (
  <Card size="sm" style={{ maxWidth: 420 }}>
    <CardHeader>
      <CardTitle>worker:v2</CardTitle>
      <CardDescription>Built 2026-09-11 09:47 · sha256:9ac71e3f52</CardDescription>
      <CardAction>
        <Badge variant="secondary">Pending</Badge>
      </CardAction>
    </CardHeader>
    <CardContent style={{ color: "var(--muted-foreground)" }}>
      Scheduled for builder on the next iteration. Runtime settings are unchanged.
    </CardContent>
  </Card>
);

export const ContentOnly = () => (
  <Card style={{ maxWidth: 420 }}>
    <CardContent>
      <div style={{ fontWeight: 500 }}>No built images on this host.</div>
      <div style={{ color: "var(--muted-foreground)" }}>
        Build one from a directory containing Tariboyfile.yaml.
      </div>
    </CardContent>
  </Card>
);
