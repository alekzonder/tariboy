import {
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "tariboy-ui";

const IMAGES = ["worker:v1", "worker:v2", "reviewer:v3", "bare:latest"];

export const Closed = () => (
  <div style={{ display: "grid", gap: 4, width: 288 }}>
    <Label htmlFor="agent-image">Agent image</Label>
    <Select defaultValue="worker:v1">
      <SelectTrigger id="agent-image" aria-label="Agent image">
        <SelectValue placeholder="Select image" />
      </SelectTrigger>
      <SelectContent>
        {IMAGES.map((ref) => (
          <SelectItem key={ref} value={ref}>
            {ref}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  </div>
);

export const Open = () => (
  <div style={{ display: "grid", gap: 4, width: 288 }}>
    <Label htmlFor="agent-image-open">Agent image</Label>
    <Select defaultValue="worker:v1" open>
      <SelectTrigger id="agent-image-open" aria-label="Agent image">
        <SelectValue placeholder="Select image" />
      </SelectTrigger>
      <SelectContent>
        {IMAGES.map((ref) => (
          <SelectItem key={ref} value={ref}>
            {ref}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  </div>
);

export const Placeholder = () => (
  <div style={{ display: "grid", gap: 4, width: 288 }}>
    <Label htmlFor="harness">Harness</Label>
    <Select>
      <SelectTrigger id="harness" aria-label="Harness">
        <SelectValue placeholder="image default" />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="claude">claude</SelectItem>
        <SelectItem value="codex">codex</SelectItem>
      </SelectContent>
    </Select>
  </div>
);

export const Disabled = () => (
  <div style={{ display: "grid", gap: 4, width: 288 }}>
    <Label htmlFor="host">Host</Label>
    <Select defaultValue="local" disabled>
      <SelectTrigger id="host" aria-label="Host">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="local">This daemon (local)</SelectItem>
      </SelectContent>
    </Select>
  </div>
);
