import type { TaskNotification } from "@/lib/tasks"

export const CUSTOMER_QUESTION_RETRY_MS = 3000

export function customerQuestionAttentionKey(hostId: string, agentName: string): string {
  return JSON.stringify([hostId, agentName])
}

export function requestingAgent(notification: TaskNotification): string | null {
  if (notification.read_at || notification.dismissed_at || notification.type !== "task.question") return null
  return notification.requesting_principal.startsWith("agent:")
    ? notification.requesting_principal.slice("agent:".length)
    : null
}
