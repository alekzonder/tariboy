type StatusVariant = "default" | "secondary" | "destructive";

// Map a backend iteration status to a badge variant. Backend Status is one of
// running|done|no_i_am_done|harness_error|timeout|killed (internal/agent/
// agent.go): a completed-clean run reads as 'default', the failure states stand
// out as 'destructive', and in-flight/incomplete ('running', 'no_i_am_done')
// stay neutral 'secondary'.
const STATUS_VARIANT: Record<string, StatusVariant> = {
  done: "default",
  harness_error: "destructive",
  timeout: "destructive",
  killed: "destructive",
};

export function statusVariant(status: string): StatusVariant {
  return Object.hasOwn(STATUS_VARIANT, status) ? STATUS_VARIANT[status] : "secondary";
}
