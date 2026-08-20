import { useState } from "react";
import { Copy, Download } from "lucide-react";
import { toast } from "sonner";
import { fetchAuditArchive, fetchAuditMarkdown } from "@/lib/auditExport";
import { Button } from "@/components/ui/button";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";

function filenamePart(value: string): string {
  return value.replace(/[^A-Za-z0-9._-]/g, "_") || "audit";
}

export function AuditExportActions({ name, iteration }: { name: string; iteration?: string }) {
  const [copying, setCopying] = useState(false);
  const [exporting, setExporting] = useState(false);

  const copy = async () => {
    setCopying(true);
    try {
      await navigator.clipboard.writeText(await fetchAuditMarkdown(name, iteration));
      toast.success(iteration ? "Iteration audit copied" : "Full audit copied");
    } catch (error) {
      toast.error(`Could not copy audit: ${String(error)}`);
    } finally {
      setCopying(false);
    }
  };

  const download = async () => {
    setExporting(true);
    try {
      const blob = await fetchAuditArchive(name, iteration);
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = `${filenamePart(name)}-${filenamePart(iteration || "all")}-audit.zip`;
      link.click();
      URL.revokeObjectURL(url);
      toast.success("Audit archive downloaded");
    } catch (error) {
      toast.error(`Could not export audit: ${String(error)}`);
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="flex items-center gap-1">
      <Button type="button" variant="outline" size="xs" aria-label="Copy audit log" disabled={copying} onClick={() => void copy()}>
        <Copy /> {copying ? "Copying…" : "Copy"}
      </Button>
      <AlertDialog>
        <AlertDialogTrigger asChild>
          <Button type="button" variant="outline" size="xs" aria-label="Export audit log"><Download /> Export</Button>
        </AlertDialogTrigger>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Export sensitive audit data?</AlertDialogTitle>
            <AlertDialogDescription>
              The ZIP contains readable Markdown and lossless JSONL. It may include prompts, reasoning, commands,
              tool arguments/results, model responses, and user data. Inspect it before sharing.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={exporting}>Cancel</AlertDialogCancel>
            <AlertDialogAction disabled={exporting} onClick={() => void download()}>{exporting ? "Preparing…" : "Download ZIP"}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
