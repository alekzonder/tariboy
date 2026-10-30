import { Link } from "react-router-dom";
import { MoreHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useStoreActions, type StoreActions } from "./useStoreActions";
import { useStoreData, type StoreData } from "./useStoreData";
import StoreImagesTable from "./StoreImagesTable";
import StoreWorkflowsTable from "./StoreWorkflowsTable";

interface CardParts {
  name: string;
  data: StoreData;
  actions: StoreActions;
}

function RemoveStoreDialog({ name, actions }: Omit<CardParts, "data">) {
  const { busy, removeError } = actions;
  return (
    <AlertDialog open={actions.removeOpen} onOpenChange={actions.changeRemoveOpen}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove Store {name}?</AlertDialogTitle>
          <AlertDialogDescription>
            Removes this registration and its managed clone. It preserves local sources and built
            images.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {removeError && (
          <p role="alert" className="text-sm text-destructive">
            {removeError}
          </p>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={busy === "remove"}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={busy === "remove"}
            onClick={(event) => {
              event.preventDefault();
              void actions.remove();
            }}
          >
            {busy === "remove" ? "Removing…" : "Remove store"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function StoreMenu({ name, data, actions }: CardParts) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          size="icon-xs"
          variant="ghost"
          aria-label={`Store actions ${name}`}
          disabled={Boolean(actions.busy) || data.loading}
        >
          <MoreHorizontal aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={() => void actions.refresh()}>Refresh</DropdownMenuItem>
        <DropdownMenuItem
          className="text-destructive"
          onSelect={() => actions.changeRemoveOpen(true)}
        >
          Remove store…
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function CardHeader({ name, basePath, linked, source, data, actions }: CardParts & CardProps) {
  const { detail } = data;
  const images = detail?.images.length;
  const shownSource = detail?.source ?? source;
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="min-w-0">
        <h2 className="font-medium">
          {linked ? (
            <Link
              className="text-primary hover:underline"
              to={`${basePath}/${encodeURIComponent(name)}`}
            >
              {name}
            </Link>
          ) : (
            name
          )}
        </h2>
        {shownSource && (
          <p className="truncate text-xs text-muted-foreground" title={detail?.path}>
            {shownSource}
            {detail && ` · ${images} ${images === 1 ? "image" : "images"}`}
          </p>
        )}
      </div>
      <div className="flex shrink-0 items-center gap-2">
        {actions.busy === "refresh" && (
          <span className="text-xs text-muted-foreground">Refreshing…</span>
        )}
        <StoreMenu name={name} data={data} actions={actions} />
      </div>
    </div>
  );
}

function AutomaticBuilds({ data, actions }: Omit<CardParts, "name">) {
  const { busy } = actions;
  const images = data.detail?.images ?? [];
  return (
    <details className="rounded border px-3 py-2">
      <summary className="cursor-pointer text-sm font-medium">Automatic builds</summary>
      <div className="mt-3 space-y-3">
        <p className="text-xs text-muted-foreground">
          Automatic builds refresh this Store every interval in minutes and rebuild the checked
          images that need an update, publishing image_version and latest. An interval of 0 disables
          them.
        </p>
        <label className="flex items-center gap-2 text-sm">
          Interval, minutes
          <Input
            aria-label="Automatic build interval"
            className="w-28"
            type="number"
            min={0}
            step={1}
            placeholder="0"
            value={data.autoInterval}
            disabled={Boolean(busy)}
            onChange={(event) => data.setAutoInterval(event.target.value)}
          />
        </label>
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
          {images.map((image) => (
            <label key={image.name} className="flex items-center gap-1.5 font-mono">
              <input
                type="checkbox"
                aria-label={`Automatic builds for ${image.name}`}
                checked={data.autoImages.includes(image.name)}
                disabled={Boolean(busy) || Boolean(image.error)}
                onChange={(event) =>
                  data.setAutoImages((current) =>
                    event.target.checked
                      ? [...current, image.name]
                      : current.filter((selected) => selected !== image.name),
                  )
                }
              />
              {image.name}
            </label>
          ))}
        </div>
        <Button
          size="sm"
          variant="outline"
          disabled={Boolean(busy)}
          onClick={() => void actions.saveAuto()}
        >
          {busy === "auto" ? "Saving…" : "Save automatic builds"}
        </Button>
      </div>
    </details>
  );
}

interface CardProps {
  name: string;
  basePath: string;
  /** On the Sources list: the name links to the Store's own page. */
  linked?: boolean;
  /** The source the list reports, shown until the detail loads. */
  source?: string;
  /** Runs after a removal; without it the page returns to the list. */
  onRemoved?: () => void;
}

/** One Store: its header with Refresh and Remove, its images and workflows,
 *  and its automatic builds. The card reads its own detail. */
export default function StoreCard(props: CardProps) {
  const { name, basePath, onRemoved } = props;
  const data = useStoreData(name);
  const actions = useStoreActions(name, basePath, data, onRemoved);
  const { detail } = data;
  return (
    <section aria-label={`Store ${name}`} className="space-y-3 rounded border p-4">
      <CardHeader {...props} data={data} actions={actions} />
      {data.loading && (
        <p role="status" className="text-sm text-muted-foreground">
          Loading Store…
        </p>
      )}
      {detail && (
        <>
          <StoreImagesTable
            images={detail.images}
            busy={actions.busy}
            build={actions.build}
            buildErrors={actions.buildErrors}
            operationHref={actions.operationHref}
          />
          {(detail.workflows?.length ?? 0) > 0 && (
            <div className="space-y-2">
              <h3 className="text-sm font-medium">Workflows</h3>
              <StoreWorkflowsTable
                workflows={detail.workflows}
                workflowImages={data.workflowImages}
                busy={actions.busy}
                buildWorkflow={actions.buildWorkflow}
              />
            </div>
          )}
          <AutomaticBuilds data={data} actions={actions} />
        </>
      )}
      {data.error && (
        <p role="alert" className="text-sm text-destructive">
          {data.error}
        </p>
      )}
      {actions.status && (
        <p role="status" className="text-sm text-muted-foreground">
          {actions.status}
        </p>
      )}
      <RemoveStoreDialog name={name} actions={actions} />
    </section>
  );
}
