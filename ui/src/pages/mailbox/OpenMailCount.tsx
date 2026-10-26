import { ApiProvider } from "@/components/ApiProvider";
import { useApi } from "@/components/apiContext";
import { useApiQuery } from "@/hooks/useApiQuery";
import { targetFor } from "@/lib/terminalsHost";
import { mailQueries, useLiveMail } from "./mailQueries";

function Count() {
  const api = useApi();
  useLiveMail("");
  const open = useApiQuery(mailQueries.unread(api, "")).data?.open ?? 0;
  if (open === 0) return null;
  const label = `${open} open letter${open === 1 ? "" : "s"}`;
  return (
    <span
      role="img"
      aria-label={label}
      title={label}
      className="ml-1.5 rounded-full bg-primary px-1.5 text-caption font-medium leading-4 text-primary-foreground"
    >
      {open}
    </span>
  );
}

/** The customer's open letters on one server, kept live by the mail socket. */
export default function OpenMailCount({ hostId }: { hostId: string }) {
  return (
    <ApiProvider target={targetFor(hostId)}>
      <Count />
    </ApiProvider>
  );
}
