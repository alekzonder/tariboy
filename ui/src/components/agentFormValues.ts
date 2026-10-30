// Values shared by the agent-create surfaces (single-agent form and group wizard).

export interface EnvRow {
  key: string;
  value: string;
}

// serializeEnv turns the key/value repeater into the daemon's `K=V,K=V` string,
// dropping rows with a blank key. Values keep their content verbatim.
export function serializeEnv(rows: EnvRow[]): string {
  return rows
    .filter((r) => r.key.trim() !== "")
    .map((r) => `${r.key.trim()}=${r.value}`)
    .join(",");
}

// commaFieldError guards the env/plugins wire format. The daemon parses both the
// `K=V,K=V` env string and the comma-list of plugins by splitting on ',' with no
// escaping (parseKV / parseList in internal/commands/agents.go, shared with the
// CLI --env/--plugins flags). A comma inside an env key/value or a plugin name
// would therefore be mis-split into extra bogus pairs. The format simply cannot
// represent it, so the create forms reject such input up front with a clear
// message instead of serializing a string the daemon silently corrupts. Returns
// null when every field is safe.
export function commaFieldError(rows: EnvRow[], plugins: string[]): string | null {
  for (const r of rows) {
    if (r.key.trim() === "") continue;
    if (r.key.includes(",")) return `env key "${r.key.trim()}": commas are not supported`;
    if (r.value.includes(",")) return `env value for "${r.key.trim()}": commas are not supported`;
  }
  const badPlugin = plugins.find((p) => p.includes(","));
  if (badPlugin) return `plugin "${badPlugin}": commas are not supported`;
  return null;
}
