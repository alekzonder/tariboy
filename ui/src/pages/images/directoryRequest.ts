export function directoryRequest(
  path: string,
  name: string,
  tag: string,
): { path: string; name: string; tag?: string } {
  return {
    path: path.trim(),
    name: name.trim(),
    ...(tag.trim() && tag.trim() !== "latest" ? { tag: tag.trim() } : {}),
  };
}
