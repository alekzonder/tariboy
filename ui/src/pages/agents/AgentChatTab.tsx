import { useSearchParams } from "react-router-dom";
import ChannelsPage from "@/pages/ChannelsPage";
import AgentChat from "./chat/AgentChat";
import { useUiMode } from "@/lib/uiMode";

// One section for both halves of an agent's messaging: the conversation the
// customer holds with it, and the raw channels it is subscribed to. They share
// a section because they are the same bus read two ways.
const VIEWS = [
  ["chat", "Chat"],
  ["channels", "Channels"],
] as const;
type View = typeof VIEWS[number][0];

function isView(value: string | null): value is View {
  return VIEWS.some(([key]) => key === value);
}

export default function AgentChatTab({ hostId = "" }: { hostId?: string }) {
  const [params, setParams] = useSearchParams();
  const simple = useUiMode() === "simple";
  const requested = params.get("view");
  const view: View = isView(requested) ? requested : "chat";
  const pick = (key: View) => {
    const next = new URLSearchParams(params);
    next.set("view", key);
    setParams(next, { replace: true });
  };
  /* The switch is rendered into the chat's own toolbar rather than above it,
     so the section keeps one 48px control row instead of stacking two. */
  const segment = (
    <div role="group" aria-label="Messaging view" className="flex shrink-0 gap-0.5 rounded-[9px] bg-muted p-0.5">
      {VIEWS.map(([key, label]) => (
        <button
          key={key}
          type="button"
          aria-pressed={view === key}
          onClick={() => pick(key)}
          className={`h-[22px] rounded-[7px] px-[9px] text-[11.5px] ${view === key
            ? "bg-card font-medium text-foreground shadow-[var(--raise)]"
            : "text-muted-foreground hover:text-foreground"}`}
        >
          {label}
        </button>
      ))}
    </div>
  );
  // Simple is the personal chat alone: no Channels, no other chats.
  if (simple) return <AgentChat hostId={hostId} personalOnly />;
  if (view === "chat") return <AgentChat hostId={hostId} leading={segment} />;
  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex h-12 shrink-0 items-center pl-4">{segment}</div>
      <div className="min-h-0 flex-1">
        <ChannelsPage />
      </div>
    </div>
  );
}
