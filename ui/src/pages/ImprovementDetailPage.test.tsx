import { afterEach, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import ImprovementDetailPage from "./ImprovementDetailPage";

afterEach(() => vi.restoreAllMocks());

it("shows the exact plan and approves its displayed revision", async () => {
  const detail = { proposal: { id: "proposal-1", judge_run_id: "run-1", revision_hash: "sha256:revision", status: "awaiting_plan_approval", draft: { subject_ids: ["subject-1"], target: { repository: "images", base_commit: "91ab820", image: "reviewer", image_digest: "sha256:old" }, findings: [{ severity: "important", criterion: "ci", observation: "CI was not checked", evidence: [{ bundle_hash: "a".repeat(64), artifact: "transcript", locator: "req-17" }] }], changes: [{ file: "skills/review/SKILL.md", intent: "Require CI state" }], acceptance: ["Reviewer records CI state"], risk: "medium", rollback_image: "reviewer:v7" } }, releases: [] };
  const fetchMock = vi.fn().mockImplementation((_url: string, init?: RequestInit) => Promise.resolve({ ok: true, status: 200, text: async () => JSON.stringify({ ok: true, result: init?.method === "POST" ? { decision: "approve" } : detail }) } as Response));
  vi.stubGlobal("fetch", fetchMock);
  render(<MemoryRouter initialEntries={["/improvements/proposal-1"]}><Routes><Route path="/improvements/:id" element={<ImprovementDetailPage />} /></Routes></MemoryRouter>);
  await screen.findByText("CI was not checked");
  expect(screen.getByText("skills/review/SKILL.md")).toBeInTheDocument();
  expect(screen.getByText("sha256:revision")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "Approve exact plan" }));
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("/api/improvements/proposal-1/plan/approve", expect.objectContaining({ method: "POST", body: JSON.stringify({ revision: "sha256:revision", reason: "" }) })));
});
