import { apiRawOn, getActiveDaemon } from "@/lib/api";

function exportPath(name: string, iteration?: string, format?: "markdown"): string {
  const query = new URLSearchParams();
  if (iteration) query.set("iteration", iteration);
  if (format) query.set("format", format);
  const suffix = query.toString();
  return `/api/agents/${encodeURIComponent(name)}/audit-export${suffix ? `?${suffix}` : ""}`;
}

export async function fetchAuditMarkdown(name: string, iteration?: string): Promise<string> {
  const response = await apiRawOn(getActiveDaemon(), "GET", exportPath(name, iteration, "markdown"));
  return response.text();
}

export async function fetchAuditArchive(name: string, iteration?: string): Promise<Blob> {
  const response = await apiRawOn(getActiveDaemon(), "GET", exportPath(name, iteration));
  return response.blob();
}
