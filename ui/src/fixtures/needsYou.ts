import type { NeedsYouViewRow } from "@/pages/home/needsYouModel";

/** A Needs you row, already formatted by the container. */
export const needsYouRow = (over: Partial<NeedsYouViewRow> = {}): NeedsYouViewRow => ({
  id: "task-TB-148",
  kind: "task",
  hostId: "",
  ref: "TB-148",
  label: "TB-148",
  title: "Approve the release notes for worker:v2",
  meta: "waiting for you · planner · 12 min ago",
  ...over,
});

/** The seven rows of a busy workspace: 100+ items wait, Home shows the most
 *  urgent seven. Two tasks, a group of four letters about one task, long
 *  sender names and a letter with no task. */
export const busyNeedsYou: NeedsYouViewRow[] = [
  needsYouRow({ id: "t1", ref: "TB-148", label: "TB-148" }),
  needsYouRow({
    id: "t2",
    ref: "TARI-7n7t",
    label: "TARI-7n7t",
    title: "Review the build output of the nightly release candidate of worker:v2",
    meta: "waiting for you · release-candidate-packager-for-the-long-running-nightly · 2 h ago",
  }),
  needsYouRow({
    id: "l1",
    kind: "letter",
    ref: "m1",
    label: "tariboy-image-creator",
    title: "Which image builds the docs: worker:v1 or bare:latest?",
    meta: "4 letters · Answer on TASK-nzex · 5 min ago",
  }),
  needsYouRow({
    id: "l2",
    kind: "letter",
    ref: "m2",
    label: "tariboy-release-publisher",
    title: "Publish the alpha build to the internal channel now or wait for the review?",
    meta: "Question on TARI-7n7t · 1 h ago",
  }),
  needsYouRow({
    id: "l3",
    kind: "letter",
    ref: "m3",
    label: "extra-image-creator",
    title: "The base image has a new digest",
    meta: "Answer on TASK-q2ab · 3 h ago",
  }),
  needsYouRow({
    id: "l4",
    kind: "letter",
    ref: "m4",
    label: "docs-bot",
    title: "Weekly summary",
    meta: "2 h ago",
  }),
  needsYouRow({
    id: "l5",
    kind: "letter",
    ref: "m5",
    label: "planner",
    title: "Which of the two plans do you prefer?",
    meta: "Nov 4",
  }),
];
