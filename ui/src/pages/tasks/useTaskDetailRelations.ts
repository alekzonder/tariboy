import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import type { TaskRelationType } from "@/lib/api";
import type { TaskDetailProps } from "./TaskDetailTypes";

export function useTaskDetailRelations(
  onAddRelation: TaskDetailProps["onAddRelation"],
  onDeleteRelation: TaskDetailProps["onDeleteRelation"],
) {
  const [relationType, setRelationType] = useState<TaskRelationType>("blocks");
  const [relationTarget, setRelationTarget] = useState("");
  const relationMutation = useMutation({
    mutationFn: (input: { key: string; type: TaskRelationType } | { id: number }) =>
      "id" in input ? onDeleteRelation(input.id) : onAddRelation(input.key, input.type),
    onSuccess: (_, input) => {
      if ("key" in input) setRelationTarget("");
    },
  });
  const submitRelation = () => {
    if (relationTarget.trim())
      relationMutation.mutate({ key: relationTarget.trim(), type: relationType });
  };

  return {
    relationType,
    setRelationType,
    relationTarget,
    setRelationTarget,
    relationMutation,
    submitRelation,
  };
}
