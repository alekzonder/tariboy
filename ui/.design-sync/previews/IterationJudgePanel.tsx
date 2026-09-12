import { IterationJudgePanel, MemoryRouter } from "tariboy-ui";

// IterationJudgePanel renders from the `judge` projection it is handed with the
// iteration detail, so its states are prop-driven and render for real here. It
// also polls the judge review history for the host in the route (useParams) and
// links into /servers/<host>/settings/advanced/judges/<run>, so a router has to
// be in scope; the history request resolves to nothing without a daemon, which
// leaves the panel on the handed-down projection — the same thing it shows on
// the first paint in the product.

const Route = ({ children }: { children: React.ReactNode }) => (
  <MemoryRouter initialEntries={["/servers/local/agents/builder/audit"]}>
    <div style={{ maxWidth: 660 }}>{children}</div>
  </MemoryRouter>
);

const completed = {
  run_id: "jr-20260911-0471",
  target_id: "builder-20260911182241-48",
  created_at: "2026-09-11T18:31:02Z",
  state: "completed",
  verdict: "Meets the goal",
  score: 8,
  completed: 3,
  failed: 0,
  pending: 0,
};

// A finished iteration that has already been judged: score, verdict and time in
// the header, with the button to run another review.
export const Reviewed = () => (
  <Route>
    <IterationJudgePanel
      agentName="builder"
      iterationId="builder-20260911182241-48"
      terminal
      judge={{ latest_completed: completed, active: null }}
    />
  </Route>
);

// A review in flight. Pending responses make the panel explain what it is
// waiting on (it checks the configured Judge workers' Autopilot state) and
// offer a link into the run.
export const ReviewRunning = () => (
  <Route>
    <IterationJudgePanel
      agentName="builder"
      iterationId="builder-20260911182241-49"
      terminal
      judge={{
        latest_completed: completed,
        active: {
          run_id: "jr-20260911-0472",
          target_id: "builder-20260911182241-49",
          created_at: "2026-09-11T18:44:10Z",
          state: "running",
          verdict: "",
          score: null,
          completed: 1,
          failed: 0,
          pending: 2,
        },
      }}
    />
  </Route>
);

// A never-judged iteration that has finished: the panel is just its heading and
// the action.
export const NeverReviewed = () => (
  <Route>
    <IterationJudgePanel
      agentName="packager"
      iterationId="packager-20260911190302-7"
      terminal
      judge={{ latest_completed: null, active: null }}
    />
  </Route>
);

// While the iteration is still running there is nothing to judge yet.
export const IterationStillRunning = () => (
  <Route>
    <IterationJudgePanel
      agentName="builder"
      iterationId="builder-20260911182241-50"
      terminal={false}
      judge={{ latest_completed: completed, active: null }}
    />
  </Route>
);
