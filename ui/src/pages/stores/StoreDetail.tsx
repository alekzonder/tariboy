import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import type { StoreActions } from "./useStoreActions";
import type { StoreData } from "./useStoreData";
import StoreImagesTable from "./StoreImagesTable";
import StoreWorkflowsTable from "./StoreWorkflowsTable";

interface StoreDetailProps {
  name: string;
  basePath: string;
  data: StoreData;
  actions: StoreActions;
}

function RemoveStoreDialog({ name, data, actions }: Omit<StoreDetailProps, "basePath">) {
  const { busy, removeError } = actions;
  return (
    <AlertDialog open={actions.removeOpen} onOpenChange={actions.changeRemoveOpen}>
      <AlertDialogTrigger asChild>
        <Button variant="destructive" disabled={Boolean(busy) || data.loading}>
          Remove store
        </Button>
      </AlertDialogTrigger>
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

function StoreDetailHeader({ name, basePath, data, actions }: StoreDetailProps) {
  const { detail } = data;
  return (
    <div className="space-y-1">
      <Link className="text-sm text-primary hover:underline" to={basePath}>
        ← Stores
      </Link>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h1 className="text-lg font-semibold">{name}</h1>
          {detail && (
            <>
              <p className="break-all text-sm text-muted-foreground">{detail.source}</p>
              <p className="break-all font-mono text-xs text-muted-foreground">{detail.path}</p>
            </>
          )}
        </div>
        <div className="flex shrink-0 gap-2">
          <Button
            variant="outline"
            disabled={Boolean(actions.busy) || data.loading}
            onClick={() => void actions.refresh()}
          >
            {actions.busy === "refresh" ? "Refreshing…" : "Refresh"}
          </Button>
          <RemoveStoreDialog name={name} data={data} actions={actions} />
        </div>
      </div>
    </div>
  );
}

function StoreAutoForm({ data, actions }: Pick<StoreDetailProps, "data" | "actions">) {
  const { busy } = actions;
  return (
    <>
      <div className="grid gap-3 md:grid-cols-[14rem_auto]">
        <Input
          aria-label="Automatic build interval"
          type="number"
          min={0}
          step={1}
          placeholder="0"
          value={data.autoInterval}
          disabled={Boolean(busy)}
          onChange={(event) => data.setAutoInterval(event.target.value)}
        />
        <Button variant="outline" disabled={Boolean(busy)} onClick={() => void actions.saveAuto()}>
          {busy === "auto" ? "Saving…" : "Save automatic builds"}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        Automatic builds refresh this Store every interval in minutes and rebuild the checked images
        that need an update, publishing image_version and latest. An interval of 0 disables them.
      </p>
    </>
  );
}

export default function StoreDetail(props: StoreDetailProps) {
  const { data, actions } = props;
  const { detail } = data;
  return (
    <>
      <StoreDetailHeader {...props} />
      {data.loading && (
        <p role="status" className="text-sm text-muted-foreground">
          Loading Store…
        </p>
      )}
      {detail && (
        <section className="space-y-2">
          <h2 className="font-medium">Images</h2>
          <Tabs defaultValue="agent">
            <TabsList>
              <TabsTrigger value="agent">Agent images</TabsTrigger>
              <TabsTrigger value="workflow">Workflow images</TabsTrigger>
            </TabsList>
            <TabsContent value="agent" className="space-y-2">
              <StoreAutoForm data={data} actions={actions} />
              <StoreImagesTable
                images={detail.images}
                autoImages={data.autoImages}
                setAutoImages={data.setAutoImages}
                busy={actions.busy}
                build={actions.build}
              />
            </TabsContent>
            <TabsContent value="workflow">
              <StoreWorkflowsTable
                workflows={detail.workflows}
                workflowImages={data.workflowImages}
                busy={actions.busy}
                buildWorkflow={actions.buildWorkflow}
              />
            </TabsContent>
          </Tabs>
        </section>
      )}
    </>
  );
}
