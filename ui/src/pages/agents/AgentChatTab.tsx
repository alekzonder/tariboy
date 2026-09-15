import { useSearchParams } from "react-router-dom";
import ChannelsPage from "@/pages/ChannelsPage";
import AgentChat from "./chat/AgentChat";
import { Button } from "@/components/ui/button";

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
  const requested = params.get("view");
  const view: View = isView(requested) ? requested : "chat";
  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      <div className="flex flex-wrap gap-2">
        {VIEWS.map(([key, label]) => (
          <Button
            key={key}
            size="sm"
            variant={view === key ? "secondary" : "ghost"}
            aria-pressed={view === key}
            onClick={() => {
              const next = new URLSearchParams(params);
              next.set("view", key);
              setParams(next, { replace: true });
            }}
          >
            {label}
          </Button>
        ))}
      </div>
      <div className="min-h-0 flex-1">
        {view === "chat" ? <AgentChat hostId={hostId} /> : <ChannelsPage />}
      </div>
    </div>
  );
}
