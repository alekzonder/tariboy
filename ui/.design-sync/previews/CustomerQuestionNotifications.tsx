import { AgentNameContext, AliasEditor, CustomerQuestionNotifications, DaemonProvider, MemoryRouter } from "tariboy-ui";

// CustomerQuestionNotifications is HEADLESS. It mounts one HostQuestionWatcher
// per registered daemon (each returns null), polls every host's task-notification
// inbox over HTTP + the tasks socket, raises native desktop notifications for
// newly seen customer questions, and publishes the resulting "needs attention"
// key set through CustomerQuestionNotificationsContext. Its only DOM output is
// `{children}` — pages such as TerminalsPage and AgentWorkspace read the context
// and draw the attention dots.
//
// So there is no visual surface to preview. This cell mounts the real component
// in the providers App gives it (DaemonProvider + a router) around a real
// shipped child, which proves the pass-through renders — but everything visible
// belongs to the child, not to this component. Graded needs-work for that
// reason rather than fabricating an attention-badge lookalike; the honest fix is
// a preview of a consumer once one lands in the bundle. See
// .design-sync/learnings/applied-c.md.

export const HeadlessPassthrough = () => (
  <MemoryRouter>
    <DaemonProvider>
      <CustomerQuestionNotifications>
        <AgentNameContext.Provider value="builder">
          <AliasEditor />
        </AgentNameContext.Provider>
      </CustomerQuestionNotifications>
    </DaemonProvider>
  </MemoryRouter>
);
