import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { ChevronDown } from "lucide-react";
import type { ApiClient, ImageSummary } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { SearchInput } from "@/components/ui/search-input";
import { errorText } from "@/lib/errors";
import { subscribeImagesChanged } from "@/lib/imageEvents";
import { navPath } from "@/lib/navRoutes";
import { relativeTime } from "@/lib/time";
import { ImageArchiveImport } from "./built/ImageArchiveImport";
import ImageBuildFromDirectory from "./ImageBuildFromDirectory";
import {
  libraryImageRows,
  unusedCount,
  visibleImageRows,
  type LibraryImageRow,
} from "./libraryImages";

type AddDialog = "build" | "import" | null;

/** The images of the route host, re-read when an image changes. Rows of a
 *  previous route host are never shown for the current one. */
function useImageList() {
  const api = useApi();
  const [listed, setListed] = useState<{ api: ApiClient; images: ImageSummary[] } | null>(null);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);

  useEffect(() => subscribeImagesChanged(() => setRevision((value) => value + 1)), []);
  useEffect(() => {
    let alive = true;
    void api.images
      .listImages()
      .then((result) => {
        if (!alive) return;
        setListed({ api, images: result.images ?? [] });
        setError("");
      })
      .catch((err) => {
        if (alive) setError(errorText(err));
      });
    return () => {
      alive = false;
    };
  }, [api, revision]);

  const images = listed?.api === api ? listed.images : null;
  return { images, error, reload: () => setRevision((value) => value + 1) };
}

/** The Library's Images tab: one row per image name. */
export default function LibraryImagesPage({ hostId }: { hostId: string }) {
  const { images, error, reload } = useImageList();
  const [query, setQuery] = useState("");
  const [showUnused, setShowUnused] = useState(false);
  const [dialog, setDialog] = useState<AddDialog>(null);
  const rows = useMemo(() => libraryImageRows(images ?? []), [images]);
  const visible = visibleImageRows(rows, { query, showUnused });

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div data-toolbar className="flex shrink-0 flex-nowrap items-center gap-3 px-4 py-2">
        <SearchInput
          className="h-7 w-55 min-w-0 shrink"
          inputClassName="text-ui"
          aria-label="Search images"
          placeholder="Search images"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <label className="flex shrink-0 items-center gap-1.5 text-body-sm whitespace-nowrap">
          <input
            type="checkbox"
            checked={showUnused}
            onChange={(event) => setShowUnused(event.target.checked)}
          />
          Show unused ({unusedCount(rows)})
        </label>
        <AddImageMenu onOpen={setDialog} />
      </div>
      <div className="min-h-0 flex-1 overflow-auto px-4 pb-4">
        {error ? (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        ) : images === null ? (
          <p className="text-sm text-muted-foreground">Loading…</p>
        ) : (
          <ImageRowsTable
            rows={visible}
            hostId={hostId}
            empty={rows.length === 0 ? "No built images on this host." : undefined}
            onShowUnused={showUnused ? undefined : () => setShowUnused(true)}
          />
        )}
      </div>
      <AddImageDialog
        dialog={dialog}
        hostId={hostId}
        onClose={() => setDialog(null)}
        onImported={reload}
      />
    </div>
  );
}

function AddImageMenu({ onOpen }: { onOpen: (dialog: AddDialog) => void }) {
  const navigate = useNavigate();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button size="xs" className="ml-auto shrink-0">
          Add image
          <ChevronDown aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => onOpen("build")}>Build from directory</DropdownMenuItem>
        <DropdownMenuItem onSelect={() => onOpen("import")}>Import archive</DropdownMenuItem>
        <DropdownMenuItem onSelect={() => navigate(navPath("library-sources", ""))}>
          Get from a source
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function AddImageDialog({
  dialog,
  hostId,
  onClose,
  onImported,
}: {
  dialog: AddDialog;
  hostId: string;
  onClose: () => void;
  onImported: () => void;
}) {
  return (
    <Dialog
      open={dialog !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <DialogContent className="max-h-[calc(100vh-2rem)] overflow-y-auto sm:max-w-3xl">
        <DialogTitle className="sr-only">
          {dialog === "import" ? "Import archive" : "Build from directory"}
        </DialogTitle>
        {dialog === "build" && <ImageBuildFromDirectory key={hostId} />}
        {dialog === "import" && (
          <ImageArchiveImport
            hostId={hostId}
            onImported={() => {
              onImported();
              onClose();
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

const COLUMNS = ["Image", "Version", "Source", "Used by", "Built", ""];

function ImageRowsTable({
  rows,
  hostId,
  empty,
  onShowUnused,
}: {
  rows: LibraryImageRow[];
  hostId: string;
  /** The message when the host has no images at all. */
  empty?: string;
  /** Shows unused images; absent when they already show. */
  onShowUnused?: () => void;
}) {
  return (
    <table className="w-full text-sm">
      <thead className="text-left text-xs text-muted-foreground">
        <tr>
          {COLUMNS.map((label) => (
            <th key={label} className="px-3 py-2 font-medium">
              {label}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <ImageRow key={row.name} row={row} hostId={hostId} />
        ))}
        {rows.length === 0 && (
          <tr>
            <td colSpan={COLUMNS.length} className="px-3 py-8 text-center text-muted-foreground">
              {empty ?? (
                <span className="inline-flex items-center gap-2">
                  {onShowUnused ? "No agent uses these images yet." : "No images match."}
                  {onShowUnused && (
                    <Button size="xs" variant="outline" onClick={onShowUnused}>
                      Show unused
                    </Button>
                  )}
                </span>
              )}
            </td>
          </tr>
        )}
      </tbody>
    </table>
  );
}

function ImageRow({ row, hostId }: { row: LibraryImageRow; hostId: string }) {
  return (
    <tr className="border-t">
      <td className="px-3 py-2">
        <Link
          className="font-mono text-primary hover:underline"
          to={`/library/images/${encodeURIComponent(row.name)}`}
        >
          {row.name}
        </Link>
      </td>
      <td className="px-3 py-2">{row.version || "—"}</td>
      <td className="px-3 py-2 text-muted-foreground">{row.source || "—"}</td>
      <td className="px-3 py-2 whitespace-nowrap">
        {row.usedBy} {row.usedBy === 1 ? "agent" : "agents"}
      </td>
      <td className="px-3 py-2 whitespace-nowrap text-muted-foreground" title={row.builtAt}>
        {relativeTime(row.builtAt)}
      </td>
      <td className="px-3 py-2 text-right">
        <Button size="xs" variant="outline" asChild>
          <Link
            aria-label={`Run agent ${row.name}`}
            to={`/?new=1&host=${encodeURIComponent(hostId)}&image=${encodeURIComponent(row.runRef)}`}
          >
            Run agent
          </Link>
        </Button>
      </td>
    </tr>
  );
}
