import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

/** The Tasks toolbar's Filters menu, from its button through a submenu row. */
async function openFiltersSub(name: RegExp) {
  await userEvent.click(screen.getByRole("button", { name: /^Filters/ }));
  await userEvent.click(await screen.findByRole("menuitem", { name }));
}

/** Picks an assignee: "Anyone", "My tasks", or an agent's name. */
export async function pickTaskAssignee(name: string) {
  await openFiltersSub(/^Assignee/);
  await userEvent.click(await screen.findByRole("menuitemradio", { name }));
}

/** Picks the queue whose name starts with `prefix`; "All" for every queue. */
export async function pickTaskQueue(prefix: string) {
  await openFiltersSub(/^Queue/);
  await userEvent.click(
    await screen.findByRole("menuitemradio", { name: new RegExp(`^${prefix}`) }),
  );
}

export async function openManageQueues() {
  await openFiltersSub(/^Queue/);
  await userEvent.click(await screen.findByRole("menuitem", { name: "Manage queues…" }));
}

export async function toggleWaitingForMe() {
  await userEvent.click(screen.getByRole("button", { name: /^Filters/ }));
  await userEvent.click(await screen.findByRole("menuitemcheckbox", { name: "Waiting for me" }));
}
