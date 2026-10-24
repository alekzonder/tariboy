import { useEffect, useReducer, useState } from "react";
import type { ImageRow } from "@/lib/api";
import { useApi } from "@/components/apiContext";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { GroupWizardGroupSection } from "@/components/groupWizard/GroupWizardGroupSection";
import { GroupWizardAgentRow } from "@/components/groupWizard/GroupWizardAgentRow";
import { useGroupWizardSubmit } from "@/components/groupWizard/useGroupWizardSubmit";
import {
  allowsPluginOverrides,
  canCreate,
  groupWizardReducer,
  initialWizardState,
} from "@/components/groupWizard/groupWizardModel";

// GroupWizard is the V4 multi-section group builder. Section 1 is the group
// (name + base cwd); the group lead is the row marked leader. Section 2 is a
// repeatable list of agents. "Create" is pure front-end orchestration: one
// POST /api/groups followed by one POST /api/agents per row (N+1 calls, in
// order). Partial failure is not rolled back — each row shows its own
// created/failed state and only failed rows are retried (the group already
// exists by then, so retry never re-creates it).
export function GroupWizard({ onCreated }: { onCreated?: () => void }) {
  const [images, setImages] = useState<ImageRow[]>([]);
  const [name, setName] = useState("");
  const [baseCwd, setBaseCwd] = useState("");
  const [state, dispatch] = useReducer(groupWizardReducer, undefined, initialWizardState);
  const { agents, leaderIdx, rowStatus } = state;
  // The host is fixed when the wizard opens: a later host switch never sends the
  // rest of a half-created group somewhere else.
  const current = useApi();
  const [api] = useState(() => current);
  const submit = useGroupWizardSubmit({ api, state, dispatch, name, baseCwd, images, onCreated });
  const { busy, leaderName } = submit;

  useEffect(() => {
    void api.images
      .listImages()
      .then((r) => setImages(r.images ?? []))
      .catch(() => setImages([]));
  }, [api]);

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-base">New group (wizard)</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <GroupWizardGroupSection
          name={name}
          onNameChange={setName}
          baseCwd={baseCwd}
          onBaseCwdChange={setBaseCwd}
          leaderName={leaderName}
        />

        <div className="space-y-3">
          {agents.map((a, i) => (
            <GroupWizardAgentRow
              key={a.id}
              agent={a}
              index={i}
              isLeader={leaderIdx === i}
              status={rowStatus[a.id]}
              images={images}
              allowsPlugins={allowsPluginOverrides(images, a.image)}
              canRemove={agents.length > 1}
              onSetLeader={() => dispatch({ type: "setLeader", idx: i })}
              onRemove={() => dispatch({ type: "removeAgent", id: a.id, idx: i })}
              onPatch={(patch) => dispatch({ type: "patchAgent", id: a.id, patch })}
            />
          ))}
          <Button
            variant="outline"
            size="sm"
            type="button"
            onClick={() => dispatch({ type: "addAgent" })}
          >
            + Add agent
          </Button>
        </div>

        {submit.formErr && <p className="text-sm text-destructive">{submit.formErr}</p>}
        {submit.groupErr && (
          <p className="text-sm text-destructive">group create failed: {submit.groupErr}</p>
        )}

        <div className="flex gap-2">
          <Button disabled={!canCreate(name, agents, leaderIdx) || busy} onClick={submit.create}>
            {busy ? "Creating…" : "Create group"}
          </Button>
          {submit.anyFailed && !busy && (
            <Button variant="outline" onClick={submit.retryFailed}>
              Retry failed
            </Button>
          )}
        </div>
        {submit.groupCreated && (
          <p className="text-xs text-muted-foreground">
            group <span className="font-mono">{name.trim()}</span> created; agents joined via their
            group field.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
